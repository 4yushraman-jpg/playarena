package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Repository provides data access for the auth domain.
// It holds both the query wrapper (for simple reads/writes) and the pool
// (for transaction-based operations such as token rotation and email verification).
type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

func NewRepository(queries *db.Queries, pool *pgxpool.Pool) *Repository {
	return &Repository{queries: queries, pool: pool}
}

// ---- user operations --------------------------------------------------------

// RegisterTxParams bundles all state needed to create a user and their first
// email-verification token in a single atomic transaction.
type RegisterTxParams struct {
	UserParams  db.CreateUserParams
	TokenHash   string
	TokenExpiry pgtype.Timestamptz
}

// RegisterTransaction atomically creates the user row and the email verification
// token in a single transaction. If either write fails the entire operation is
// rolled back, preventing orphaned accounts that can never be verified.
//
// Unique-constraint violations on email / username are mapped to typed errors.
func (r *Repository) RegisterTransaction(ctx context.Context, p RegisterTxParams) (*db.User, *db.EmailVerificationToken, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	user, err := qtx.CreateUser(ctx, p.UserParams)
	if err != nil {
		if pgutil.IsUniqueViolation(err, "uq_users_email") {
			return nil, nil, ErrEmailAlreadyRegistered
		}
		if pgutil.IsUniqueViolation(err, "uq_users_username") {
			return nil, nil, ErrUsernameAlreadyTaken
		}
		return nil, nil, err
	}

	token, err := qtx.CreateEmailVerificationToken(ctx, db.CreateEmailVerificationTokenParams{
		UserID:    user.ID,
		TokenHash: p.TokenHash,
		ExpiresAt: p.TokenExpiry,
	})
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return &user, &token, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*db.User, error) {
	user, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetUserByID(ctx context.Context, userID pgtype.UUID) (*db.User, error) {
	user, err := r.queries.GetUserByID(ctx, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// ---- refresh token operations -----------------------------------------------

func (r *Repository) CreateRefreshToken(ctx context.Context, params db.CreateRefreshTokenParams) (*db.RefreshToken, error) {
	token, err := r.queries.CreateRefreshToken(ctx, params)
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *Repository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*db.RefreshToken, error) {
	token, err := r.queries.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	return &token, nil
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, tokenID pgtype.UUID) error {
	return r.queries.RevokeRefreshToken(ctx, tokenID)
}

func (r *Repository) RevokeUserRefreshTokens(ctx context.Context, userID pgtype.UUID) error {
	return r.queries.RevokeUserRefreshTokens(ctx, userID)
}

// RotateRefreshToken revokes the existing token and atomically issues a new one
// inside a single serializable transaction.
//
// Replay detection: if the incoming token was already revoked, all active
// tokens for that user are immediately revoked and ErrTokenReuse is returned.
func (r *Repository) RotateRefreshToken(
	ctx context.Context,
	oldHash string,
	newParams db.CreateRefreshTokenParams,
) (*db.RefreshToken, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	oldToken, err := qtx.GetRefreshTokenByHashForUpdate(ctx, oldHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	if oldToken.RevokedAt.Valid {
		_ = qtx.RevokeUserRefreshTokens(ctx, oldToken.UserID)
		_ = tx.Commit(ctx)
		return nil, ErrTokenReuse
	}

	if oldToken.ExpiresAt.Time.Before(time.Now()) {
		return nil, ErrExpiredToken
	}

	if err := qtx.RevokeRefreshToken(ctx, oldToken.ID); err != nil {
		return nil, err
	}

	newToken, err := qtx.CreateRefreshToken(ctx, newParams)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &newToken, nil
}

// ---- email verification token operations ------------------------------------

func (r *Repository) GetEmailVerificationTokenByHash(ctx context.Context, hash string) (*db.EmailVerificationToken, error) {
	token, err := r.queries.GetEmailVerificationTokenByHash(ctx, hash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrVerificationTokenInvalid
		}
		return nil, err
	}
	return &token, nil
}

// VerifyEmailTransaction atomically marks the verification token as used and
// transitions the user account from pending_verification to active.
// Both writes succeed or both roll back.
func (r *Repository) VerifyEmailTransaction(ctx context.Context, tokenID, userID pgtype.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	if err := qtx.UseEmailVerificationToken(ctx, tokenID); err != nil {
		return err
	}

	if err := qtx.VerifyUserEmail(ctx, userID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// DeleteExpiredEmailVerificationTokens removes expired tokens. Called by the
// background cleanup job.
func (r *Repository) DeleteExpiredEmailVerificationTokens(ctx context.Context, before pgtype.Timestamptz) error {
	return r.queries.DeleteExpiredEmailVerificationTokens(ctx, before)
}

// ---- RBAC operations --------------------------------------------------------

func (r *Repository) GetUserRolesByOrganization(ctx context.Context, params db.GetUserRolesByOrganizationParams) ([]db.Role, error) {
	return r.queries.GetUserRolesByOrganization(ctx, params)
}

func (r *Repository) GetUserPlatformRoles(ctx context.Context, userID pgtype.UUID) ([]db.Role, error) {
	return r.queries.GetUserPlatformRoles(ctx, userID)
}

func (r *Repository) GetUserOrganizations(ctx context.Context, userID pgtype.UUID) ([]db.Organization, error) {
	return r.queries.GetUserOrganizations(ctx, userID)
}
