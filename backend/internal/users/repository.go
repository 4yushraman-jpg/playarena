package users

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Repository provides data access for the users domain.
type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(queries *db.Queries, pool *pgxpool.Pool) *Repository {
	return &Repository{queries: queries, pool: pool}
}

// ── reads ─────────────────────────────────────────────────────────────────────

func (r *Repository) GetByID(ctx context.Context, id pgtype.UUID) (*db.User, error) {
	user, err := r.queries.GetUserByID(ctx, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *Repository) ListUsers(ctx context.Context, params ListParams) ([]db.User, error) {
	return r.queries.ListUsersPaginated(ctx, db.ListUsersPaginatedParams{
		LimitCount:  params.Limit,
		OffsetCount: params.Offset,
	})
}

func (r *Repository) CountUsers(ctx context.Context) (int64, error) {
	return r.queries.CountUsers(ctx)
}

// ── UpdateProfileTransaction ──────────────────────────────────────────────────

// UpdateProfileTxParams bundles everything needed by UpdateProfileTransaction.
type UpdateProfileTxParams struct {
	ActorID pgtype.UUID
	Params  db.UpdateUserProfileParams
}

// UpdateProfileTransaction atomically:
//  1. Acquires a FOR UPDATE lock on the user row.
//  2. Re-validates the user is still active (closes the TOCTOU window between
//     the service-layer status check and this write).
//  3. Performs the profile UPDATE.
//  4. Appends an audit record (skipped when no profile field actually changed).
//
// new_data is derived from the DB row returned by the UPDATE so the audit
// always reflects what was actually stored.
func (r *Repository) UpdateProfileTransaction(ctx context.Context, p UpdateProfileTxParams) (*db.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	// Lock the user row so no concurrent deactivation can slip through between
	// the service-layer status check and this write.
	locked, err := qtx.GetUserByIDForUpdate(ctx, p.Params.ID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if locked.Status != db.UserStatusActive {
		return nil, ErrForbidden
	}

	oldData, err := json.Marshal(profileSnapshot(&locked))
	if err != nil {
		return nil, fmt.Errorf("users: marshal old profile snapshot: %w", err)
	}

	updated, err := qtx.UpdateUserProfile(ctx, p.Params)
	if err != nil {
		if pgutil.IsUniqueViolation(err, "uq_users_username") {
			return nil, ErrUsernameAlreadyTaken
		}
		return nil, err
	}

	newData, err := json.Marshal(profileSnapshot(&updated))
	if err != nil {
		return nil, fmt.Errorf("users: marshal new profile snapshot: %w", err)
	}

	// Skip the audit record when no profile field actually changed.
	if !bytes.Equal(oldData, newData) {
		if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
			UserID:     p.ActorID,
			Action:     db.AuditActionUpdate,
			EntityType: "users",
			EntityID:   p.Params.ID,
			OldData:    oldData,
			NewData:    newData,
		}); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &updated, nil
}

// ── ChangePasswordTransaction ─────────────────────────────────────────────────

// ChangePasswordTxParams bundles everything needed by ChangePasswordTransaction.
// newPasswordHash must be computed by the caller (bcrypt) BEFORE the transaction
// begins so the CPU-heavy hash does not hold an idle DB connection.
type ChangePasswordTxParams struct {
	UserID          pgtype.UUID
	NewPasswordHash string
}

// ChangePasswordTransaction atomically:
//  1. Acquires a FOR UPDATE lock on the user row.
//  2. Re-validates the user is still active (rejects suspended/inactive).
//  3. Replaces the password hash.
//  4. Revokes all active refresh tokens (Case 3 semantics — no successor_id).
//  5. Appends an audit record.
//
// The FOR UPDATE lock serialises concurrent password-change attempts and closes
// the TOCTOU window between the bcrypt verify (outside) and this write.
func (r *Repository) ChangePasswordTransaction(ctx context.Context, p ChangePasswordTxParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	// Lock the user row so no concurrent status change can slip through.
	locked, err := qtx.GetUserByIDForUpdate(ctx, p.UserID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrInvalidCredentials
		}
		return err
	}
	if locked.Status != db.UserStatusActive {
		// User was suspended or deactivated between the bcrypt verify and now.
		// Return ErrInvalidCredentials to avoid leaking account status.
		return ErrInvalidCredentials
	}

	if err := qtx.UpdateUserPasswordHash(ctx, db.UpdateUserPasswordHashParams{
		PasswordHash: p.NewPasswordHash,
		ID:           p.UserID,
	}); err != nil {
		return err
	}

	if err := qtx.RevokeUserRefreshTokens(ctx, p.UserID); err != nil {
		return err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		UserID:     p.UserID,
		Action:     db.AuditActionUpdate,
		EntityType: "users",
		EntityID:   p.UserID,
		OldData:    []byte(`{"password_changed":false,"sessions_revoked":false}`),
		NewData:    []byte(`{"password_changed":true,"sessions_revoked":true}`),
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ── DeactivateTransaction ─────────────────────────────────────────────────────

// DeactivateTxParams bundles the actor and target for DeactivateTransaction.
type DeactivateTxParams struct {
	ActorID  pgtype.UUID
	TargetID pgtype.UUID
}

// DeactivateTransaction atomically:
//  1. Acquires a FOR UPDATE lock on the platform_admin role row (sentinel) to
//     serialise all concurrent deactivations before any admin-status check.
//  2. Checks whether the target holds a platform_admin grant (under the lock).
//  3. If so: counts remaining active admins; returns ErrLastPlatformAdmin if ≤ 1.
//  4. Locks the target user row (FOR UPDATE).
//  5. Guards against already-inactive targets.
//  6. Sets status = inactive; asserts exactly one row was affected.
//  7. Revokes all active refresh tokens.
//  8. Appends an audit record.
//
// Acquiring the sentinel first (step 1) closes the race where two concurrent
// deactivations of the last two platform admins both observe count=2 and both
// proceed, leaving zero administrators.
func (r *Repository) DeactivateTransaction(ctx context.Context, p DeactivateTxParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	// Step 1: Acquire the sentinel lock on the platform_admin role row first.
	// This serialises all concurrent DeactivateTransactions before any admin
	// check occurs, eliminating the race where two concurrent deactivations of
	// the last two admins both observe count=2 before either acquires the lock.
	if _, err := qtx.GetPlatformAdminRoleForUpdate(ctx); err != nil {
		return fmt.Errorf("users: platform_admin role not found: %w", err)
	}

	// Step 2: Is the target a platform admin? (checked under the sentinel lock)
	isPlatformAdmin, err := qtx.IsUserActivePlatformAdmin(ctx, p.TargetID)
	if err != nil {
		return err
	}

	// Step 3: Last-admin guard — only needed when target is a platform admin.
	if isPlatformAdmin {
		count, err := qtx.CountActivePlatformAdmins(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastPlatformAdmin
		}
	}

	// Step 4: Lock the target user row.
	locked, err := qtx.GetUserByIDForUpdate(ctx, p.TargetID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrUserNotFound
		}
		return err
	}

	// Step 5: Guard against already-inactive targets.
	if locked.Status == db.UserStatusInactive {
		return ErrUserAlreadyInactive
	}

	// Step 6: Deactivate. Assert exactly one row was affected.
	rowsAffected, err := qtx.DeactivateUser(ctx, p.TargetID)
	if err != nil {
		return err
	}
	if rowsAffected != 1 {
		return fmt.Errorf("users: DeactivateUser affected %d rows, expected 1", rowsAffected)
	}

	// Step 7: Revoke all active sessions.
	if err := qtx.RevokeUserRefreshTokens(ctx, p.TargetID); err != nil {
		return err
	}

	// Step 8: Audit.
	oldData, err := json.Marshal(struct {
		Status string `json:"status"`
	}{Status: string(locked.Status)})
	if err != nil {
		return fmt.Errorf("users: marshal deactivate audit old_data: %w", err)
	}
	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		UserID:     p.ActorID,
		Action:     db.AuditActionUpdate,
		EntityType: "users",
		EntityID:   p.TargetID,
		OldData:    oldData,
		NewData:    []byte(`{"status":"inactive"}`),
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
