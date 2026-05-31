package auth

import (
	"context"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

// Repository provides data access for the auth domain.
// It holds both the query wrapper (for simple reads/writes) and the pool
// (for transaction-based operations such as token rotation).
type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

func NewRepository(queries *db.Queries, pool *pgxpool.Pool) *Repository {
	return &Repository{
		queries: queries,
		pool:    pool,
	}
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
// This forces the legitimate user to re-authenticate, limiting the window of
// exposure when a stolen token is replayed.
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

	// Lock the row to prevent concurrent rotation of the same token.
	oldToken, err := qtx.GetRefreshTokenByHashForUpdate(ctx, oldHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	// Replay detection: token was already revoked.
	if oldToken.RevokedAt.Valid {
		// Best-effort: invalidate every active session for this user.
		_ = qtx.RevokeUserRefreshTokens(ctx, oldToken.UserID)
		_ = tx.Commit(ctx)
		return nil, ErrTokenReuse
	}

	// Expiry check inside the transaction for consistency.
	if oldToken.ExpiresAt.Time.Before(time.Now()) {
		return nil, ErrExpiredToken
	}

	// Revoke the consumed token.
	if err := qtx.RevokeRefreshToken(ctx, oldToken.ID); err != nil {
		return nil, err
	}

	// Issue the replacement token.
	newToken, err := qtx.CreateRefreshToken(ctx, newParams)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &newToken, nil
}

// ---- user operations --------------------------------------------------------

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

// ---- helpers ----------------------------------------------------------------

// parseNetIP converts a *netip.Addr suitable for CreateRefreshTokenParams.
// The field is already *netip.Addr in the generated struct so this is a
// pass-through; it exists to make nil-safety explicit at call sites.
func parseNetIP(addr *netip.Addr) *netip.Addr { return addr }
