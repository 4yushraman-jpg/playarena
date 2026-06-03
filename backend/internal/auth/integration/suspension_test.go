package auth_integration_test

import (
	"context"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestLogin_PendingVerification verifies that a user who has not verified their
// email address cannot log in and receives 403 with the verification error.
func TestLogin_PendingVerification(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user, _ := fixtures.CreatePendingUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    user.Email,
		"password": fixtures.KnownPasswordRaw,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "email address not verified")
}

// TestLogin_Suspended verifies that a suspended user cannot log in and
// receives 403 "account suspended".
func TestLogin_Suspended(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateSuspendedUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    user.Email,
		"password": fixtures.KnownPasswordRaw,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "account suspended")
}

// TestLogin_Inactive verifies that an inactive user cannot log in and
// receives 403 "account inactive".
func TestLogin_Inactive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateInactiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    user.Email,
		"password": fixtures.KnownPasswordRaw,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "account inactive")
}

// TestSuspension_BlocksLogin is an alias scenario confirming the same
// suspended-status behaviour under a suspension that happens at test setup
// time (as opposed to mid-flight). Kept separate from TestLogin_Suspended to
// make the "suspension" intent explicit in the test name.
func TestSuspension_BlocksLogin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	// Create active user then suspend before attempting login.
	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	if _, err := testPool.Exec(ctx,
		"UPDATE users SET status = 'suspended' WHERE id = $1", user.ID,
	); err != nil {
		t.Fatalf("suspend user: %v", err)
	}

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    user.Email,
		"password": fixtures.KnownPasswordRaw,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "account suspended")
}

// TestSuspension_BlocksMidFlightRefresh verifies that suspending a user after
// they have obtained a valid refresh token prevents them from using it.
//
// This test exercises the service-layer pre-check: service.Refresh calls
// assertUserActive(user) before entering RotateRefreshToken. Because the
// suspension is committed to the DB before the refresh request is issued,
// the pre-check observes the suspended status and returns ErrUserSuspended.
//
// Step 3b (the in-transaction user-status re-check inside RotateRefreshToken,
// added by Phase 13A Auth Hardening v2) is defense-in-depth for the race where
// a suspension is committed AFTER the service pre-check but BEFORE the rotation
// transaction commits. That sub-millisecond window cannot be deterministically
// exercised in a sequential integration test; it is an accepted architectural
// constraint, not a gap in this test.
func TestSuspension_BlocksMidFlightRefresh(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Obtain a refresh token while the user is still active.
	_, refreshToken := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)

	// Suspend the user between token issuance and refresh.
	if _, err := testPool.Exec(ctx,
		"UPDATE users SET status = 'suspended' WHERE id = $1", user.ID,
	); err != nil {
		t.Fatalf("suspend user: %v", err)
	}

	// Refresh must now be blocked by the in-transaction status re-check.
	resp := ts.post(t, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": refreshToken,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "account suspended")
}
