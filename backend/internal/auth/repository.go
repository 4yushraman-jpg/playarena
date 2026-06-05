package auth

import (
	"context"
	"fmt"
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

// CreateEmailVerificationToken inserts a new single-use verification token for
// an existing user. Used by ResendVerification to issue a replacement token
// when the original expired or was not delivered. The old token remains in the
// database and is valid until it expires or is consumed — concurrent tokens
// for the same user are allowed by the schema.
func (r *Repository) CreateEmailVerificationToken(ctx context.Context, params db.CreateEmailVerificationTokenParams) error {
	_, err := r.queries.CreateEmailVerificationToken(ctx, params)
	return err
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

// LogoutTransaction revokes a single refresh token inside a transaction that
// holds a FOR UPDATE row lock on the token.
//
// This closes the race condition in the previous two-step approach
// (GetRefreshTokenByHash → RevokeRefreshToken) where a concurrent rotation
// could slip between the read and the revoke, leaving the rotated successor
// token active despite the user's intent to log out.
//
// Under the lock:
//   - If the token is already revoked (by a prior rotation or logout), the
//     function returns success — logout is idempotent.
//   - If valid, it sets revoked_at without touching successor_id, preserving
//     Case 3 semantics: any later presentation triggers ErrTokenReuse.
func (r *Repository) LogoutTransaction(ctx context.Context, tokenHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	token, err := qtx.GetRefreshTokenByHashForUpdate(ctx, tokenHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrInvalidToken
		}
		return err
	}

	if token.RevokedAt.Valid {
		// Already revoked (concurrent rotation or duplicate logout) — idempotent.
		return tx.Commit(ctx)
	}

	if err := qtx.RevokeRefreshToken(ctx, token.ID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// RotateRefreshToken revokes the existing token and atomically issues a new one
// inside a single transaction.
//
// Replay detection is deterministic and structural — no time windows, no wall-clock
// comparisons. The successor_id column on the old token encodes its history:
//
//	Case 1: revoked_at IS NULL
//	        Active token. Proceed with rotation.
//
//	Case 2: revoked_at IS NOT NULL, successor_id IS NOT NULL
//	        Token was already rotated. This is either a concurrent duplicate
//	        request from the same client, a client retry, or a stolen token
//	        being replayed after the legitimate rotation already happened.
//	        Return ErrInvalidToken — do NOT wipe sessions. The legitimate
//	        client still holds the successor token.
//
//	Case 3: revoked_at IS NOT NULL, successor_id IS NULL
//	        Token was explicitly revoked: logout, logout-all, password reset.
//	        Presenting it here is anomalous. Revoke every active session for
//	        the user and return ErrTokenReuse.
//
// Auth Hardening v2: after confirming Case 1, the user's current status is
// re-checked inside the transaction (READ COMMITTED). This check is
// database-backed and occurs much closer to the commit than the pre-check in
// service.Refresh, closing the window where a suspension could be missed.
//
// Transaction order: the new token is inserted before the old token is revoked
// so that the new token's ID is available as the successor link in
// RevokeAndLinkSuccessor. The rows-affected count on that UPDATE is asserted to
// be exactly 1; any other count is an internal invariant violation.
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

	// Step 2: Acquire an exclusive row lock on the old token. No concurrent
	// transaction can modify this row until we commit or roll back.
	oldToken, err := qtx.GetRefreshTokenByHashForUpdate(ctx, oldHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	// Step 3: Validate state matrix.
	if oldToken.RevokedAt.Valid {
		if oldToken.SuccessorID.Valid {
			// Case 2: already rotated — concurrent request, retry, or replay of a
			// rotated token. No session wipe; the holder of the successor is legitimate.
			return nil, ErrInvalidToken
		}
		// Case 3: explicitly revoked. Revoke all remaining sessions and signal reuse.
		if err := qtx.RevokeUserRefreshTokens(ctx, oldToken.UserID); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, ErrTokenReuse
	}

	// Step 3b: Re-validate user status inside the transaction (Auth Hardening v2).
	// Uses READ COMMITTED — sees the latest committed user state at the time of
	// this read, which is much closer to the commit than the service-layer pre-check.
	user, err := qtx.GetUserByID(ctx, oldToken.UserID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	if err := assertUserActive(&user); err != nil {
		return nil, err
	}

	// Step 4: Validate expiry.
	if oldToken.ExpiresAt.Time.Before(time.Now()) {
		return nil, ErrExpiredToken
	}

	// Step 5: Insert the new token first so its ID is available for the link.
	newToken, err := qtx.CreateRefreshToken(ctx, newParams)
	if err != nil {
		return nil, err
	}

	// Step 6: Atomically revoke the old token and record the successor ID.
	rowsAffected, err := qtx.RevokeAndLinkSuccessor(ctx, db.RevokeAndLinkSuccessorParams{
		ID:          oldToken.ID,
		SuccessorID: newToken.ID,
	})
	if err != nil {
		return nil, err
	}

	// Step 7: Defensive invariant. The FOR UPDATE lock guarantees this row was
	// not modified between the lock acquisition and this UPDATE. Any count other
	// than 1 is a bug in this function.
	if rowsAffected != 1 {
		return nil, fmt.Errorf("auth: RevokeAndLinkSuccessor affected %d rows, expected 1", rowsAffected)
	}

	// Step 8: Commit.
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

// VerifyEmailTransaction atomically validates and consumes the verification
// token, then transitions the user account from pending_verification to active.
//
// All validity checks (used_at, expires_at) are performed inside the
// transaction after acquiring a FOR UPDATE row lock on the token. This closes
// the TOCTOU window that existed when checks ran outside the transaction.
func (r *Repository) VerifyEmailTransaction(ctx context.Context, tokenHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	token, err := qtx.GetEmailVerificationTokenByHashForUpdate(ctx, tokenHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrVerificationTokenInvalid
		}
		return err
	}

	if token.UsedAt.Valid {
		return ErrVerificationTokenUsed
	}
	if token.ExpiresAt.Time.Before(time.Now()) {
		return ErrVerificationTokenExpired
	}

	if err := qtx.UseEmailVerificationToken(ctx, token.ID); err != nil {
		return err
	}
	if err := qtx.VerifyUserEmail(ctx, token.UserID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// DeleteExpiredEmailVerificationTokens removes expired tokens. Called by the
// background cleanup scheduler.
func (r *Repository) DeleteExpiredEmailVerificationTokens(ctx context.Context, before pgtype.Timestamptz) error {
	return r.queries.DeleteExpiredEmailVerificationTokens(ctx, before)
}

// ---- password reset token operations ----------------------------------------

// ForgotPasswordTxParams bundles the state for creating a password reset token
// and its associated audit record in a single transaction.
type ForgotPasswordTxParams struct {
	UserID    pgtype.UUID
	TokenHash string
	ExpiresAt pgtype.Timestamptz
}

// ForgotPasswordTransaction atomically creates the reset token and writes a
// "password_reset_requested" audit record. Both writes succeed or both roll back.
func (r *Repository) ForgotPasswordTransaction(ctx context.Context, p ForgotPasswordTxParams) (*db.PasswordResetToken, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	token, err := qtx.CreatePasswordResetToken(ctx, db.CreatePasswordResetTokenParams{
		UserID:    p.UserID,
		TokenHash: p.TokenHash,
		ExpiresAt: p.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		UserID:     token.UserID,
		Action:     db.AuditActionCreate,
		EntityType: "password_reset_tokens",
		EntityID:   token.ID,
		NewData:    []byte(`{"requested":true}`),
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &token, nil
}

// ResetPasswordTransaction atomically resets a user's password.
//
// Deadlock-safe lock ordering (Fix #1):
//
// The previous design locked only the presented token row, then called
// UseAllUserPasswordResetTokens which had to UPDATE all sibling rows. Two
// concurrent resets each holding a different token would each wait for the
// other's locked row → cycle deadlock.
//
// The new design:
//  1. Non-locking pre-fetch to obtain the user_id for the lock-all step.
//  2. Begin transaction.
//  3. Lock ALL outstanding tokens for the user in ascending id order — the
//     same deterministic sequence for every concurrent caller. One transaction
//     blocks instead of both locking disjoint rows and waiting on each other.
//  4. Find the target token inside the locked set.
//  5. Validate expiry.
//  6. Mark token and all siblings used.
//  7. Update password, revoke sessions, write audit record.
//  8. Commit.
//
// On any failure the transaction rolls back; the caller may present a new
// reset link without losing their account.
func (r *Repository) ResetPasswordTransaction(ctx context.Context, tokenHash, newPasswordHash string) error {
	// Step 1: Non-locking fetch — retrieves user_id and token.ID before the
	// transaction starts. This is safe: validation happens again inside the
	// transaction under locks. The only purpose here is to obtain the user_id
	// needed for the deterministic lock-all query in Step 3.
	preview, err := r.queries.GetPasswordResetTokenByHash(ctx, tokenHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrResetTokenInvalid
		}
		return err
	}

	// Step 2: Begin transaction.
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	// Step 3: Lock all outstanding tokens for this user in deterministic
	// ascending-id order. Both concurrent callers lock the same set in the
	// same order → the second blocks instead of deadlocking.
	locked, err := qtx.LockUserPasswordResetTokens(ctx, preview.UserID)
	if err != nil {
		return err
	}

	// Step 4: Find the target token inside the locked set.
	// If not present, it was consumed by a concurrent reset between Step 1
	// and Step 3 (UseAllUserPasswordResetTokens set used_at, removing it
	// from the WHERE used_at IS NULL filter).
	var token db.PasswordResetToken
	found := false
	for _, t := range locked {
		if t.ID == preview.ID {
			token = t
			found = true
			break
		}
	}
	if !found {
		return ErrResetTokenUsed
	}

	// Step 5: Validate expiry. used_at IS NULL is guaranteed by the
	// LockUserPasswordResetTokens WHERE clause.
	if token.ExpiresAt.Time.Before(time.Now()) {
		return ErrResetTokenExpired
	}

	// Step 6a: Mark this specific token as used.
	if err := qtx.UsePasswordResetToken(ctx, token.ID); err != nil {
		return err
	}

	// Step 6b: Invalidate all other outstanding reset tokens for the user.
	// Safe: we hold locks on all of them in deterministic order (Step 3).
	if err := qtx.UseAllUserPasswordResetTokens(ctx, token.UserID); err != nil {
		return err
	}

	// Step 7: Replace the password hash.
	if err := qtx.UpdateUserPasswordHash(ctx, db.UpdateUserPasswordHashParams{
		PasswordHash: newPasswordHash,
		ID:           token.UserID,
	}); err != nil {
		return err
	}

	// Step 8: Revoke all active refresh tokens. successor_id is not set —
	// these tokens are explicitly revoked (Case 3). Any subsequent refresh
	// attempt with them triggers a full session wipe.
	if err := qtx.RevokeUserRefreshTokens(ctx, token.UserID); err != nil {
		return err
	}

	// Step 9: Audit record. Both old_data and new_data required for update action.
	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		UserID:     token.UserID,
		Action:     db.AuditActionUpdate,
		EntityType: "users",
		EntityID:   token.UserID,
		OldData:    []byte(`{"password_changed":true}`),
		NewData:    []byte(`{"password_changed":true,"sessions_revoked":true}`),
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// DeleteExpiredPasswordResetTokens removes tokens past their expiry. Called by
// the background cleanup scheduler.
func (r *Repository) DeleteExpiredPasswordResetTokens(ctx context.Context, before pgtype.Timestamptz) error {
	return r.queries.DeleteExpiredPasswordResetTokens(ctx, before)
}

// equalizeEnumerationTiming opens a no-op transaction whose query profile
// matches the successful ForgotPasswordTransaction path, preventing timing-based
// email enumeration.
//
// Without equalization an attacker can distinguish registered from unregistered
// addresses by measuring response latency: the "not found" path avoids all DB
// writes and completes several milliseconds faster. Even though the response
// body and HTTP status are identical, a persistent adversary can achieve
// statistically significant enumeration from response-time distributions alone.
//
// Query profile alignment:
//
//	ForgotPasswordTransaction: BEGIN  INSERT(token)  INSERT(audit)  COMMIT  = 4 round-trips
//	equalizeEnumerationTiming: BEGIN  SELECT 1       SELECT NOW()   COMMIT  = 4 round-trips
//
// Two read-only queries replace the two writes to match the round-trip count.
// They carry no side effects and cannot fail in ways that would alter timing.
// The previous single-SELECT version produced only 3 round-trips, leaving a
// ~1–2 ms gap measurable at sufficient request volume.
//
// Failure handling: all errors are intentionally swallowed. If the pool is
// exhausted or the DB is temporarily unreachable, the forgot-password response
// is still delivered promptly — the correct fail-open behaviour for a
// timing-equalization measure.
func (r *Repository) equalizeEnumerationTiming(ctx context.Context) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var dummy int
	_ = tx.QueryRow(ctx, "SELECT 1").Scan(&dummy)
	_ = tx.QueryRow(ctx, "SELECT NOW()").Scan(new(interface{}))

	_ = tx.Commit(ctx)
}

// equalizeResendVerificationTiming equalises the response time for
// ResendVerification failure paths that do not perform the INSERT step.
//
// Round-trip profile comparison:
//
//	not-found / already-active: GetUserByEmail(1) + this(5) = 6 total
//	success:                    GetUserByEmail(1) + INSERT(1) + equalizeEnumerationTiming(4) = 6 total
//
// The extra SELECT 1 inside the transaction accounts for the INSERT round-trip
// on the success path, producing equal totals across all outcomes and preventing
// timing-based enumeration of pending-verification accounts.
func (r *Repository) equalizeResendVerificationTiming(ctx context.Context) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var dummy int
	_ = tx.QueryRow(ctx, "SELECT 1").Scan(&dummy)
	_ = tx.QueryRow(ctx, "SELECT NOW()").Scan(new(interface{}))
	_ = tx.QueryRow(ctx, "SELECT 1").Scan(&dummy) // matches INSERT round-trip on success path

	_ = tx.Commit(ctx)
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
