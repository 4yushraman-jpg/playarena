package users

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Service implements the users domain use-cases.
type Service struct {
	repo *Repository
}

// NewService constructs a Service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// ── authorization guards ──────────────────────────────────────────────────────

// assertSelfOrAdmin returns nil when the actor is the target user (self-access)
// or holds a platform-level token (admin). Returns ErrForbidden otherwise.
func assertSelfOrAdmin(actorID, targetID string, actorIsAdmin bool) error {
	if actorIsAdmin || actorID == targetID {
		return nil
	}
	return ErrForbidden
}

// assertPlatformAdmin returns nil when actorIsAdmin is true.
func assertPlatformAdmin(actorIsAdmin bool) error {
	if !actorIsAdmin {
		return ErrForbidden
	}
	return nil
}

// ── public operations ─────────────────────────────────────────────────────────

// GetByID fetches a user profile by ID.
// Allowed: self (actorID == targetID) or platform admin.
func (s *Service) GetByID(ctx context.Context, actorID string, actorIsAdmin bool, targetID string) (*UserResponse, error) {
	uid, err := pgutil.ParseUUID(targetID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	if err := assertSelfOrAdmin(actorID, targetID, actorIsAdmin); err != nil {
		return nil, err
	}

	user, err := s.repo.GetByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	return userToResponse(user), nil
}

// UpdateProfile applies a PATCH to a user's editable profile fields.
// Allowed: self or platform admin. Email is immutable and rejected if supplied.
func (s *Service) UpdateProfile(ctx context.Context, actorID string, actorIsAdmin bool, targetID string, req UpdateProfileRequest) (*UserResponse, error) {
	targetUID, err := pgutil.ParseUUID(targetID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, ErrForbidden
	}

	if err := assertSelfOrAdmin(actorID, targetID, actorIsAdmin); err != nil {
		return nil, err
	}

	if req.Email != nil {
		return nil, ErrEmailNotUpdatable
	}

	current, err := s.repo.GetByID(ctx, targetUID)
	if err != nil {
		return nil, err
	}

	// Fast-path rejection; the transaction re-checks under a FOR UPDATE lock
	// and is the authoritative guard against concurrent deactivation.
	if current.Status != db.UserStatusActive {
		return nil, ErrForbidden
	}

	// Merge PATCH fields over current state, validating each provided field.
	params := db.UpdateUserProfileParams{
		ID:          targetUID,
		FirstName:   current.FirstName,
		LastName:    current.LastName,
		Username:    current.Username,
		Phone:       current.Phone,
		DateOfBirth: current.DateOfBirth,
		Gender:      current.Gender,
	}

	if req.FirstName != nil {
		v := strings.TrimSpace(*req.FirstName)
		if len(v) < 1 || len(v) > 100 {
			return nil, badRequest("first_name", "must be between 1 and 100 characters")
		}
		params.FirstName = v
	}

	if req.LastName != nil {
		v := strings.TrimSpace(*req.LastName)
		if len(v) < 1 || len(v) > 100 {
			return nil, badRequest("last_name", "must be between 1 and 100 characters")
		}
		params.LastName = v
	}

	if req.Username != nil {
		v := strings.TrimSpace(*req.Username)
		if len([]rune(v)) < 3 || len([]rune(v)) > 30 {
			return nil, badRequest("username", "must be between 3 and 30 characters")
		}
		for _, c := range v {
			if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' {
				return nil, badRequest("username", "must contain only letters, numbers, and underscores")
			}
		}
		params.Username = v
	}

	if req.Phone != nil {
		if *req.Phone == "" {
			params.Phone = nil
		} else {
			if len(*req.Phone) > 20 {
				return nil, badRequest("phone", "must be at most 20 characters")
			}
			params.Phone = req.Phone
		}
	}

	if req.DateOfBirth != nil {
		if *req.DateOfBirth == "" {
			params.DateOfBirth = pgtype.Date{}
		} else {
			t, err := time.Parse("2006-01-02", *req.DateOfBirth)
			if err != nil {
				return nil, badRequest("date_of_birth", "must be a valid date in YYYY-MM-DD format")
			}
			params.DateOfBirth = pgtype.Date{Time: t, Valid: true}
		}
	}

	if req.Gender != nil {
		if *req.Gender == "" {
			params.Gender = nil
		} else {
			g := db.Gender(*req.Gender)
			switch g {
			case db.GenderMale, db.GenderFemale, db.GenderOther, db.GenderPreferNotToSay:
				params.Gender = &g
			default:
				return nil, badRequest("gender", "must be one of: male, female, other, prefer_not_to_say")
			}
		}
	}

	updated, err := s.repo.UpdateProfileTransaction(ctx, UpdateProfileTxParams{
		ActorID: actorUID,
		Params:  params,
	})
	if err != nil {
		return nil, err
	}
	return userToResponse(updated), nil
}

// ChangePassword verifies the current password and replaces it.
// Self-only: even a platform admin cannot call this on behalf of another user —
// the operation requires knowing the current password. Admin-initiated resets
// use the forgot-password → reset flow instead.
func (s *Service) ChangePassword(ctx context.Context, actorID string, targetID string, req ChangePasswordRequest) error {
	if actorID != targetID {
		return ErrForbidden
	}

	uid, err := pgutil.ParseUUID(targetID)
	if err != nil {
		return ErrUserNotFound
	}

	// Fetch current user to verify password and check status.
	current, err := s.repo.GetByID(ctx, uid)
	if err != nil {
		return err
	}
	if current.Status != db.UserStatusActive {
		return ErrInvalidCredentials
	}

	// bcrypt verify — CPU-heavy, must run outside the transaction.
	if err := bcrypt.CompareHashAndPassword([]byte(current.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		return ErrInvalidCredentials
	}

	// bcrypt hash — CPU-heavy, must run outside the transaction.
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("users: hash new password: %w", err)
	}

	return s.repo.ChangePasswordTransaction(ctx, ChangePasswordTxParams{
		UserID:          uid,
		NewPasswordHash: string(newHash),
	})
}

// ListUsers returns a paginated list of all users. Platform admin only.
func (s *Service) ListUsers(ctx context.Context, actorIsAdmin bool, params ListParams) (*ListResponse, error) {
	if err := assertPlatformAdmin(actorIsAdmin); err != nil {
		return nil, err
	}

	if params.Limit <= 0 || params.Limit > MaxListLimit {
		params.Limit = DefaultListLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	users, err := s.repo.ListUsers(ctx, params)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.CountUsers(ctx)
	if err != nil {
		return nil, err
	}

	resp := make([]UserResponse, len(users))
	for i := range users {
		resp[i] = *userToResponse(&users[i])
	}
	return &ListResponse{
		Users:  resp,
		Total:  total,
		Limit:  params.Limit,
		Offset: params.Offset,
	}, nil
}

// DeactivateUser sets a user's status to inactive and revokes all their
// sessions. Platform admin only. Includes a last-admin guard.
func (s *Service) DeactivateUser(ctx context.Context, actorID string, actorIsAdmin bool, targetID string) error {
	if err := assertPlatformAdmin(actorIsAdmin); err != nil {
		return err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return ErrForbidden
	}

	targetUID, err := pgutil.ParseUUID(targetID)
	if err != nil {
		return ErrUserNotFound
	}

	return s.repo.DeactivateTransaction(ctx, DeactivateTxParams{
		ActorID:  actorUID,
		TargetID: targetUID,
	})
}

// ── internal helpers ──────────────────────────────────────────────────────────

func userToResponse(u *db.User) *UserResponse {
	resp := &UserResponse{
		ID:        pgutil.UUIDToString(u.ID),
		Email:     u.Email,
		Username:  u.Username,
		FullName:  buildFullName(u.FirstName, u.LastName),
		Status:    string(u.Status),
		CreatedAt: u.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt: u.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
	if u.Phone != nil {
		resp.Phone = u.Phone
	}
	if u.DateOfBirth.Valid {
		s := u.DateOfBirth.Time.Format("2006-01-02")
		resp.DateOfBirth = &s
	}
	if u.Gender != nil {
		s := string(*u.Gender)
		resp.Gender = &s
	}
	return resp
}

func buildFullName(firstName, lastName string) string {
	if firstName == lastName {
		return firstName
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s", firstName, lastName))
}

// profileSnapshotData is the audit log payload for profile updates.
type profileSnapshotData struct {
	FirstName   string  `json:"first_name"`
	LastName    string  `json:"last_name"`
	Username    string  `json:"username"`
	Phone       *string `json:"phone,omitempty"`
	DateOfBirth *string `json:"date_of_birth,omitempty"`
	Gender      *string `json:"gender,omitempty"`
}

func profileSnapshot(u *db.User) profileSnapshotData {
	snap := profileSnapshotData{
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Username:  u.Username,
		Phone:     u.Phone,
	}
	if u.DateOfBirth.Valid {
		s := u.DateOfBirth.Time.Format("2006-01-02")
		snap.DateOfBirth = &s
	}
	if u.Gender != nil {
		s := string(*u.Gender)
		snap.Gender = &s
	}
	return snap
}
