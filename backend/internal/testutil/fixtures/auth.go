// Package fixtures provides typed test-fixture helpers for the auth domain.
//
// Isolation model: each fixture is keyed by a random UUID so concurrent tests
// do not share rows. Call CleanupUser at the end of each test to remove all
// cascading rows for that user (refresh_tokens, password_reset_tokens, etc.).
// No transaction-per-test wrapping is used because repository code calls
// pool.Begin() internally and would need to observe its own writes.
package fixtures

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

// bcryptTestCost uses the minimum bcrypt cost for test fixtures. Cost 4 is the
// lowest value accepted by the library and runs ~10× faster than cost 12,
// keeping fixture setup time sub-millisecond. Tests that verify password
// correctness should accept any hash produced at this cost.
const bcryptTestCost = bcrypt.MinCost

// KnownPasswordRaw is the plain-text password stored in every user fixture.
// Tests that need to call VerifyPassword can compare against this value.
const KnownPasswordRaw = "TestPassword1!"

// knownPasswordHash is a cost-4 bcrypt hash of KnownPasswordRaw, computed
// once at package init to avoid repeating the hash in every fixture call.
var knownPasswordHash string

func init() {
	h, err := bcrypt.GenerateFromPassword([]byte(KnownPasswordRaw), bcryptTestCost)
	if err != nil {
		panic("fixtures: compute test password hash: " + err.Error())
	}
	knownPasswordHash = string(h)
}

// CreateActiveUser inserts a user with status=active and returns the db.User
// row. The user's email and username are randomised so concurrent tests do not
// conflict. The password hash uses bcryptTestCost (cost 4).
func CreateActiveUser(ctx context.Context, t testing.TB, pool *pgxpool.Pool) db.User {
	t.Helper()

	uid := newUUID(ctx, t, pool)
	short := uid.Bytes[0:4]
	email := fmt.Sprintf("test_%x@example.com", short)
	username := fmt.Sprintf("testuser_%x", short)

	queries := db.New(pool)
	user, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		Username:     username,
		PasswordHash: knownPasswordHash,
		FirstName:    "Test",
		LastName:     "User",
	})
	if err != nil {
		t.Fatalf("fixtures.CreateActiveUser CreateUser: %v", err)
	}

	// Activate the account — CreateUser defaults to pending_verification.
	if _, err := pool.Exec(ctx,
		"UPDATE users SET status = 'active', email_verified_at = NOW() WHERE id = $1",
		user.ID,
	); err != nil {
		t.Fatalf("fixtures.CreateActiveUser activate: %v", err)
	}
	user.Status = db.UserStatusActive

	return user
}

// CreateRefreshToken inserts a valid (active, un-revoked) refresh token for the
// given user and returns the raw token value (to be used directly by callers)
// alongside the db row.
func CreateRefreshToken(ctx context.Context, t testing.TB, pool *pgxpool.Pool, userID pgtype.UUID) (raw string, row db.RefreshToken) {
	t.Helper()

	raw = pseudoRandomToken(ctx, t, pool)
	tokenHash := HashToken(raw)

	queries := db.New(pool)
	tok, err := queries.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(7 * 24 * time.Hour), Valid: true},
		IpAddress: nil,
		UserAgent: nil,
	})
	if err != nil {
		t.Fatalf("fixtures.CreateRefreshToken: %v", err)
	}
	return raw, tok
}

// CreatePasswordResetToken inserts a valid (unused, not-yet-expired) password
// reset token for the given user and returns the raw token value.
func CreatePasswordResetToken(ctx context.Context, t testing.TB, pool *pgxpool.Pool, userID pgtype.UUID) string {
	t.Helper()

	raw := pseudoRandomToken(ctx, t, pool)

	queries := db.New(pool)
	if _, err := queries.CreatePasswordResetToken(ctx, db.CreatePasswordResetTokenParams{
		UserID:    userID,
		TokenHash: HashToken(raw),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(1 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("fixtures.CreatePasswordResetToken: %v", err)
	}
	return raw
}

// CreateExpiredPasswordResetToken inserts a password reset token that is
// already past its expiry. The token is otherwise unused (used_at IS NULL).
//
// Implementation note: the table has CHECK (expires_at > created_at), so a
// past-expiry token cannot be created via the normal INSERT path (which sets
// created_at = NOW()). Instead, we use a raw INSERT that back-dates both
// columns: created_at = NOW()-3h, expires_at = NOW()-2h. The constraint is
// satisfied (NOW()-2h > NOW()-3h) and the token is expired (expires_at < NOW()).
func CreateExpiredPasswordResetToken(ctx context.Context, t testing.TB, pool *pgxpool.Pool, userID pgtype.UUID) string {
	t.Helper()

	raw := pseudoRandomToken(ctx, t, pool)

	if _, err := pool.Exec(ctx,
		`INSERT INTO password_reset_tokens (user_id, token_hash, expires_at, created_at)
		 VALUES ($1, $2, NOW() - INTERVAL '2 hours', NOW() - INTERVAL '3 hours')`,
		userID, HashToken(raw),
	); err != nil {
		t.Fatalf("fixtures.CreateExpiredPasswordResetToken: %v", err)
	}
	return raw
}

// CleanupUser deletes the user and all cascading rows (refresh_tokens,
// password_reset_tokens, etc.). Call via t.Cleanup or defer in tests.
func CleanupUser(ctx context.Context, t testing.TB, pool *pgxpool.Pool, userID pgtype.UUID) {
	t.Helper()
	if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID); err != nil {
		t.Logf("fixtures.CleanupUser: %v", err)
	}
}

// HashToken returns the hex-encoded SHA-256 hash of the raw token value,
// matching the storage format used by auth.HashTokenForStorage. Duplicated
// here to avoid an import cycle (fixtures → auth → db → fixtures).
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// CreatePendingUser creates a user in pending_verification state and an
// associated email verification token. Returns the user row and the raw
// (unhashed) verification token so callers can exercise the verify-email flow.
func CreatePendingUser(ctx context.Context, t testing.TB, pool *pgxpool.Pool) (db.User, string) {
	t.Helper()

	uid := newUUID(ctx, t, pool)
	short := uid.Bytes[0:4]
	email := fmt.Sprintf("pending_%x@example.com", short)
	username := fmt.Sprintf("pending_%x", short)

	queries := db.New(pool)
	user, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		Username:     username,
		PasswordHash: knownPasswordHash,
		FirstName:    "Pending",
		LastName:     "User",
	})
	if err != nil {
		t.Fatalf("fixtures.CreatePendingUser CreateUser: %v", err)
	}

	raw := pseudoRandomToken(ctx, t, pool)
	if _, err := queries.CreateEmailVerificationToken(ctx, db.CreateEmailVerificationTokenParams{
		UserID:    user.ID,
		TokenHash: HashToken(raw),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("fixtures.CreatePendingUser CreateEmailVerificationToken: %v", err)
	}

	return user, raw
}

// CreateSuspendedUser creates an active user then sets their status to suspended.
func CreateSuspendedUser(ctx context.Context, t testing.TB, pool *pgxpool.Pool) db.User {
	t.Helper()
	user := CreateActiveUser(ctx, t, pool)
	if _, err := pool.Exec(ctx,
		"UPDATE users SET status = 'suspended' WHERE id = $1",
		user.ID,
	); err != nil {
		t.Fatalf("fixtures.CreateSuspendedUser: %v", err)
	}
	user.Status = db.UserStatusSuspended
	return user
}

// CreateInactiveUser creates an active user then sets their status to inactive.
func CreateInactiveUser(ctx context.Context, t testing.TB, pool *pgxpool.Pool) db.User {
	t.Helper()
	user := CreateActiveUser(ctx, t, pool)
	if _, err := pool.Exec(ctx,
		"UPDATE users SET status = 'inactive' WHERE id = $1",
		user.ID,
	); err != nil {
		t.Fatalf("fixtures.CreateInactiveUser: %v", err)
	}
	user.Status = db.UserStatusInactive
	return user
}

// CreatePlatformAdmin creates an active user and grants them the platform_admin
// role (organization_id = NULL). Cleanup cascades via CleanupUser.
func CreatePlatformAdmin(ctx context.Context, t testing.TB, pool *pgxpool.Pool) db.User {
	t.Helper()
	user := CreateActiveUser(ctx, t, pool)

	var roleID pgtype.UUID
	if err := pool.QueryRow(ctx,
		"SELECT id FROM roles WHERE slug = 'platform_admin' LIMIT 1",
	).Scan(&roleID); err != nil {
		t.Fatalf("fixtures.CreatePlatformAdmin lookup role: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO user_organization_roles (user_id, organization_id, role_id, granted_by)
		 VALUES ($1, NULL, $2, $1)
		 ON CONFLICT DO NOTHING`,
		user.ID, roleID,
	); err != nil {
		t.Fatalf("fixtures.CreatePlatformAdmin grant role: %v", err)
	}

	return user
}

// CreateOrgWithRole creates an organization and grants the given user a role
// within it. roleSlug must be one of the system roles seeded by migration 000017
// (e.g. "org_owner", "org_admin", "viewer").
//
// Returns the organization UUID as a formatted string ready for HTTP requests.
// Registers t.Cleanup to delete the organization (CASCADE removes role grants).
// User cleanup is the caller's responsibility (via CleanupUser).
func CreateOrgWithRole(ctx context.Context, t testing.TB, pool *pgxpool.Pool, userID pgtype.UUID, roleSlug string) string {
	t.Helper()

	uid := newUUID(ctx, t, pool)
	orgName := fmt.Sprintf("Test Org %x", uid.Bytes[0:4])
	orgSlug := fmt.Sprintf("test-org-%x", uid.Bytes[0:4])

	queries := db.New(pool)
	org, err := queries.CreateOrganization(ctx, db.CreateOrganizationParams{
		Name: orgName,
		Slug: orgSlug,
		Type: db.OrgTypeClub,
	})
	if err != nil {
		t.Fatalf("fixtures.CreateOrgWithRole create org: %v", err)
	}

	var roleID pgtype.UUID
	if err := pool.QueryRow(ctx,
		"SELECT id FROM roles WHERE slug = $1 LIMIT 1",
		roleSlug,
	).Scan(&roleID); err != nil {
		t.Fatalf("fixtures.CreateOrgWithRole lookup role %q: %v", roleSlug, err)
	}

	if err := queries.GrantRoleToUserInOrg(ctx, db.GrantRoleToUserInOrgParams{
		UserID:         userID,
		OrganizationID: org.ID,
		RoleID:         roleID,
		GrantedBy:      userID,
	}); err != nil {
		t.Fatalf("fixtures.CreateOrgWithRole grant role: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
	})

	return uuidStr(org.ID)
}

// CreateExpiredEmailVerificationToken inserts an already-expired email
// verification token (used_at IS NULL, expires_at in the past).
//
// The table has a CHECK constraint requiring expires_at > created_at, so both
// columns must be back-dated: created_at = NOW()-3h, expires_at = NOW()-2h
// satisfies the constraint while placing expires_at before the current time.
func CreateExpiredEmailVerificationToken(ctx context.Context, t testing.TB, pool *pgxpool.Pool, userID pgtype.UUID) string {
	t.Helper()
	raw := pseudoRandomToken(ctx, t, pool)
	if _, err := pool.Exec(ctx,
		`INSERT INTO email_verification_tokens (user_id, token_hash, expires_at, created_at)
		 VALUES ($1, $2, NOW() - INTERVAL '2 hours', NOW() - INTERVAL '3 hours')`,
		userID, HashToken(raw),
	); err != nil {
		t.Fatalf("fixtures.CreateExpiredEmailVerificationToken: %v", err)
	}
	return raw
}

// CreateExpiredRefreshToken inserts a refresh token that is already past its
// expiry. The token is otherwise active (revoked_at IS NULL, successor_id IS NULL).
func CreateExpiredRefreshToken(ctx context.Context, t testing.TB, pool *pgxpool.Pool, userID pgtype.UUID) string {
	t.Helper()
	raw := pseudoRandomToken(ctx, t, pool)
	if _, err := pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at, created_at)
		 VALUES ($1, $2, NOW() - INTERVAL '1 day', NOW() - INTERVAL '9 days')`,
		userID, HashToken(raw),
	); err != nil {
		t.Fatalf("fixtures.CreateExpiredRefreshToken: %v", err)
	}
	return raw
}

// uuidStr formats a pgtype.UUID as the standard hyphenated UUID string
// (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx).
func uuidStr(u pgtype.UUID) string {
	b := u.Bytes
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}

// ---- internal helpers -------------------------------------------------------

// newUUID generates a new UUID by querying gen_random_uuid() from PostgreSQL.
func newUUID(ctx context.Context, t testing.TB, pool *pgxpool.Pool) pgtype.UUID {
	t.Helper()
	var uid pgtype.UUID
	if err := pool.QueryRow(ctx, "SELECT gen_random_uuid()").Scan(&uid); err != nil {
		t.Fatalf("fixtures.newUUID: %v", err)
	}
	return uid
}

// pseudoRandomToken returns a hex string derived from a DB-generated UUID.
// Used as the raw token value in fixtures: unique per call, no CSPRNG needed.
func pseudoRandomToken(ctx context.Context, t testing.TB, pool *pgxpool.Pool) string {
	t.Helper()
	uid := newUUID(ctx, t, pool)
	return fmt.Sprintf("%x", uid.Bytes)
}
