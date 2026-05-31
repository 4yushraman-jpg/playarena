package auth

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
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
// Multi-tenancy rules (fixes C1, C2, C3):
//   - Platform admins (users with platform-scoped roles): omit organization_id
//     to receive a platform-level access token (OrganizationID = "").
//   - Single-org users: organization_id is optional; selected automatically.
//   - Multi-org users: organization_id is required; ErrOrganizationRequired
//     is returned with the org list when it is missing.
//
// Status rules (fixes H3):
//   - Suspended, Inactive, and PendingVerification all block login.
func (s *Service) Login(ctx context.Context, req LoginRequest, ipAddress *netip.Addr, userAgent *string) (*LoginResponse, error) {
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
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
		uuidToString(user.ID),
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
		IpAddress: parseNetIP(ipAddress),
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
// Token rotation (fixes H1): every successful call revokes the presented
// token and issues a new one. Replayed (already-revoked) tokens trigger
// immediate revocation of all user sessions.
//
// Status re-validation (fixes H2): suspended/inactive users cannot refresh.
func (s *Service) Refresh(ctx context.Context, req RefreshRequest, ipAddress *netip.Addr, userAgent *string) (*RefreshResponse, error) {
	if req.RefreshToken == "" {
		return nil, ErrInvalidToken
	}

	oldHash := HashTokenForStorage(req.RefreshToken)

	// Resolve user before rotation so we can re-validate status.
	// We peek at the token first without locking to get the user ID.
	existingToken, err := s.repo.GetRefreshTokenByHash(ctx, oldHash)
	if err != nil {
		return nil, err
	}

	// Re-validate user status on every refresh (fixes H2).
	user, err := s.repo.GetUserByID(ctx, existingToken.UserID)
	if err != nil {
		return nil, err
	}
	if err := assertUserActive(user); err != nil {
		return nil, err
	}

	orgID, role, err := s.resolveOrgContext(ctx, user.ID, req.OrganizationID)
	if err != nil {
		return nil, err
	}

	// Generate the replacement token before the transaction so we can pass it
	// atomically to RotateRefreshToken.
	newRefreshTokenRaw, err := GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	newParams := db.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: HashTokenForStorage(newRefreshTokenRaw),
		ExpiresAt: GetRefreshTokenExpiryTime(),
		IpAddress: parseNetIP(ipAddress),
		UserAgent: userAgent,
	}

	// Atomically revoke old token and insert new one (fixes H1).
	_, err = s.repo.RotateRefreshToken(ctx, oldHash, newParams)
	if err != nil {
		return nil, err
	}

	accessToken, err := GenerateAccessToken(
		uuidToString(user.ID),
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
// The corresponding access token remains valid until it expires naturally;
// its 15-minute lifetime makes this an acceptable trade-off.
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
// The AuthUser is populated from the validated access token by middleware.
func (s *Service) Me(ctx context.Context, principal *AuthUser) (*MeResponse, error) {
	uid := pgtype.UUID{}
	if err := uid.Scan(principal.UserID); err != nil {
		return nil, ErrInvalidToken
	}

	user, err := s.repo.GetUserByID(ctx, uid)
	if err != nil {
		// ErrUserNotFound here means the account was deleted after the token
		// was issued. Return ErrInvalidToken so the handler returns 401.
		if err == ErrUserNotFound {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	return &MeResponse{
		ID:       uuidToString(user.ID),
		Email:    user.Email,
		Username: user.Username,
		FullName: user.FirstName + " " + user.LastName,
		Status:   string(user.Status),
	}, nil
}

// ---- internal helpers -------------------------------------------------------

// assertUserActive returns a typed error if the account is not in a state that
// permits login. PendingVerification is treated as blocking (fixes H3).
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

// resolveOrgContext determines the organization context for a token.
//
// Strategy:
//  1. If orgIDHint is provided: validate the user has a role in that org
//     and return that org's first role slug.
//  2. If the user has platform-level grants: return platform context ("", role).
//  3. If the user has exactly one org: select it automatically.
//  4. If the user has multiple orgs and no hint: return ErrOrganizationRequired.
//
// Returns (organizationID string, roleSlug string, error).
func (s *Service) resolveOrgContext(ctx context.Context, userID pgtype.UUID, orgIDHint string) (string, string, error) {
	if orgIDHint != "" {
		return s.resolveExplicitOrg(ctx, userID, orgIDHint)
	}

	// No hint provided — try platform roles first.
	platformRoles, err := s.repo.GetUserPlatformRoles(ctx, userID)
	if err != nil {
		return "", "", err
	}
	if len(platformRoles) > 0 {
		return "", platformRoles[0].Slug, nil
	}

	// Fall back to org membership.
	orgs, err := s.repo.GetUserOrganizations(ctx, userID)
	if err != nil {
		return "", "", err
	}

	switch len(orgs) {
	case 0:
		return "", "", &ErrOrganizationRequired{Organizations: nil}
	case 1:
		return s.resolveExplicitOrg(ctx, userID, uuidToString(orgs[0].ID))
	default:
		summaries := make([]OrgSummary, len(orgs))
		for i, o := range orgs {
			summaries[i] = OrgSummary{
				ID:   uuidToString(o.ID),
				Name: o.Name,
				Slug: o.Slug,
			}
		}
		return "", "", &ErrOrganizationRequired{Organizations: summaries}
	}
}

// resolveExplicitOrg validates that the user holds at least one active role in
// the specified organization and returns the first role slug.
func (s *Service) resolveExplicitOrg(ctx context.Context, userID pgtype.UUID, orgIDStr string) (string, string, error) {
	orgUUID := pgtype.UUID{}
	if err := orgUUID.Scan(orgIDStr); err != nil {
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

// uuidToString converts a pgtype.UUID to its canonical hyphenated string form.
func uuidToString(uid pgtype.UUID) string {
	if !uid.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uid.Bytes[0:4],
		uid.Bytes[4:6],
		uid.Bytes[6:8],
		uid.Bytes[8:10],
		uid.Bytes[10:16],
	)
}
