package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Service implements the auth domain use-cases.
type Service struct {
	repo   *Repository
	config *config.Config
}

func NewService(repo *Repository, cfg *config.Config) *Service {
	return &Service{repo: repo, config: cfg}
}

// Login authenticates a user and returns a token pair.
//
// Multi-tenancy rules:
//   - Platform admins (users with platform-scoped roles): omit organization_id
//     to receive a platform-level access token (OrganizationID = "").
//   - Single-org users: organization_id is optional; selected automatically.
//   - Multi-org users: organization_id is required; ErrOrganizationRequired
//     is returned with the org list when it is missing.
//
// Status rules: Suspended, Inactive, and PendingVerification all block login.
//
// Timing note: a dummy bcrypt comparison is always performed when the email is
// not found, so the response time is indistinguishable from a wrong-password
// failure. This prevents timing-based email enumeration (H3).
func (s *Service) Login(ctx context.Context, req LoginRequest, ipAddress *netip.Addr, userAgent *string) (*LoginResponse, error) {
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		// H3 fix: equalise timing regardless of whether the email exists.
		// dummyBcryptHash was pre-computed at bcrypt cost 12 (matches real cost).
		_ = VerifyPassword(req.Password, dummyBcryptHash)
		return nil, ErrInvalidCredentials
	}

	if !VerifyPassword(req.Password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	if err := assertUserActive(user); err != nil {
		return nil, err
	}

	orgID, role, err := s.resolveOrgContext(ctx, user.ID, req.OrganizationID)
	if err != nil {
		return nil, err
	}

	accessToken, err := GenerateAccessToken(
		pgutil.UUIDToString(user.ID),
		orgID,
		role,
		user.Email,
		s.config.JWTSecret,
	)
	if err != nil {
		return nil, err
	}

	refreshTokenRaw, err := GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	_, err = s.repo.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: HashTokenForStorage(refreshTokenRaw),
		ExpiresAt: GetRefreshTokenExpiryTime(),
		IpAddress: ipAddress,
		UserAgent: userAgent,
	})
	if err != nil {
		return nil, err
	}

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenRaw,
		ExpiresIn:    int64(accessTokenDuration.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

// Refresh validates a refresh token, rotates it, re-validates user status,
// and issues a new access token + refresh token pair.
//
// Token rotation: every successful call revokes the presented token and issues
// a new one. Replayed (already-revoked) tokens trigger immediate revocation of
// all user sessions.
func (s *Service) Refresh(ctx context.Context, req RefreshRequest, ipAddress *netip.Addr, userAgent *string) (*RefreshResponse, error) {
	if req.RefreshToken == "" {
		return nil, ErrInvalidToken
	}

	oldHash := HashTokenForStorage(req.RefreshToken)

	existingToken, err := s.repo.GetRefreshTokenByHash(ctx, oldHash)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.GetUserByID(ctx, existingToken.UserID)
	if err != nil {
		// H5 fix: a deleted user with a live refresh token must return 401,
		// not 500. ErrUserNotFound is an expected condition here.
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	if err := assertUserActive(user); err != nil {
		return nil, err
	}

	orgID, role, err := s.resolveOrgContext(ctx, user.ID, req.OrganizationID)
	if err != nil {
		return nil, err
	}

	newRefreshTokenRaw, err := GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	newParams := db.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: HashTokenForStorage(newRefreshTokenRaw),
		ExpiresAt: GetRefreshTokenExpiryTime(),
		IpAddress: ipAddress,
		UserAgent: userAgent,
	}

	if _, err = s.repo.RotateRefreshToken(ctx, oldHash, newParams); err != nil {
		return nil, err
	}

	accessToken, err := GenerateAccessToken(
		pgutil.UUIDToString(user.ID),
		orgID,
		role,
		user.Email,
		s.config.JWTSecret,
	)
	if err != nil {
		return nil, err
	}

	return &RefreshResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshTokenRaw,
		ExpiresIn:    int64(accessTokenDuration.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

// Logout revokes the presented refresh token.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return ErrInvalidToken
	}

	storedToken, err := s.repo.GetRefreshTokenByHash(ctx, HashTokenForStorage(refreshToken))
	if err != nil {
		return err
	}

	return s.repo.RevokeRefreshToken(ctx, storedToken.ID)
}

// Me returns profile information for the authenticated user.
func (s *Service) Me(ctx context.Context, principal *AuthUser) (*MeResponse, error) {
	uid := pgtype.UUID{}
	if err := uid.Scan(principal.UserID); err != nil {
		return nil, ErrInvalidToken
	}

	user, err := s.repo.GetUserByID(ctx, uid)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	return &MeResponse{
		ID:       pgutil.UUIDToString(user.ID),
		Email:    user.Email,
		Username: user.Username,
		FullName: buildFullName(user.FirstName, user.LastName),
		Status:   string(user.Status),
	}, nil
}

// Register creates a new user account with status pending_verification.
// Both the user row and the email verification token are written in a single
// transaction (H1 fix): if either write fails, the entire operation rolls back
// so there are no orphaned accounts that can never be verified.
//
// NOTE: VerificationToken is returned here to allow testing without a live
// email service. The handler gates this field behind IsDevelopment() (H2 fix).
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	firstName, lastName := splitFullName(req.FullName)

	// Generate the token BEFORE the transaction so the transaction only does
	// DB work and we avoid holding the connection open during crypto ops.
	rawToken, err := GenerateVerificationToken()
	if err != nil {
		return nil, err
	}

	user, _, err := s.repo.RegisterTransaction(ctx, RegisterTxParams{
		UserParams: db.CreateUserParams{
			Email:        strings.ToLower(strings.TrimSpace(req.Email)),
			Username:     strings.TrimSpace(req.Username),
			PasswordHash: hash,
			FirstName:    firstName,
			LastName:     lastName,
		},
		TokenHash:   HashTokenForStorage(rawToken),
		TokenExpiry: GetVerificationTokenExpiryTime(),
	})
	if err != nil {
		return nil, err
	}

	return &RegisterResponse{
		ID:                pgutil.UUIDToString(user.ID),
		Email:             user.Email,
		Username:          user.Username,
		Message:           "registration successful, please verify your email address",
		VerificationToken: rawToken,
	}, nil
}

// VerifyEmail consumes a single-use verification token and activates the account.
func (s *Service) VerifyEmail(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return ErrVerificationTokenInvalid
	}

	token, err := s.repo.GetEmailVerificationTokenByHash(ctx, HashTokenForStorage(rawToken))
	if err != nil {
		return err
	}

	if token.UsedAt.Valid {
		return ErrVerificationTokenUsed
	}

	if token.ExpiresAt.Time.Before(time.Now()) {
		return ErrVerificationTokenExpired
	}

	return s.repo.VerifyEmailTransaction(ctx, token.ID, token.UserID)
}

// DeleteExpiredVerificationTokens removes tokens that have passed their expiry.
// Intended to be called from a background cleanup job (e.g. hourly cron).
func (s *Service) DeleteExpiredVerificationTokens(ctx context.Context) error {
	return s.repo.DeleteExpiredEmailVerificationTokens(ctx, pgtype.Timestamptz{
		Time:  time.Now(),
		Valid: true,
	})
}

// ---- internal helpers -------------------------------------------------------

func assertUserActive(user *db.User) error {
	switch user.Status {
	case db.UserStatusSuspended:
		return ErrUserSuspended
	case db.UserStatusInactive:
		return ErrUserInactive
	case db.UserStatusPendingVerification:
		return ErrUserPendingVerification
	}
	return nil
}

func (s *Service) resolveOrgContext(ctx context.Context, userID pgtype.UUID, orgIDHint string) (string, string, error) {
	if orgIDHint != "" {
		return s.resolveExplicitOrg(ctx, userID, orgIDHint)
	}

	platformRoles, err := s.repo.GetUserPlatformRoles(ctx, userID)
	if err != nil {
		return "", "", err
	}
	if len(platformRoles) > 0 {
		return "", platformRoles[0].Slug, nil
	}

	orgs, err := s.repo.GetUserOrganizations(ctx, userID)
	if err != nil {
		return "", "", err
	}

	switch len(orgs) {
	case 0:
		return "", "", &ErrOrganizationRequired{Organizations: nil}
	case 1:
		return s.resolveExplicitOrg(ctx, userID, pgutil.UUIDToString(orgs[0].ID))
	default:
		summaries := make([]OrgSummary, len(orgs))
		for i, o := range orgs {
			summaries[i] = OrgSummary{
				ID:   pgutil.UUIDToString(o.ID),
				Name: o.Name,
				Slug: o.Slug,
			}
		}
		return "", "", &ErrOrganizationRequired{Organizations: summaries}
	}
}

func (s *Service) resolveExplicitOrg(ctx context.Context, userID pgtype.UUID, orgIDStr string) (string, string, error) {
	orgUUID, err := pgutil.ParseUUID(orgIDStr)
	if err != nil {
		return "", "", ErrOrganizationNotFound
	}

	roles, err := s.repo.GetUserRolesByOrganization(ctx, db.GetUserRolesByOrganizationParams{
		UserID:         userID,
		OrganizationID: orgUUID,
	})
	if err != nil {
		return "", "", err
	}
	if len(roles) == 0 {
		return "", "", ErrOrganizationNotFound
	}

	return orgIDStr, roles[0].Slug, nil
}

func splitFullName(fullName string) (firstName, lastName string) {
	trimmed := strings.TrimSpace(fullName)
	if trimmed == "" {
		return "", ""
	}
	idx := strings.IndexByte(trimmed, ' ')
	if idx == -1 {
		return trimmed, trimmed
	}
	return trimmed[:idx], strings.TrimSpace(trimmed[idx+1:])
}

func buildFullName(firstName, lastName string) string {
	if firstName == lastName {
		return firstName
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s", firstName, lastName))
}
