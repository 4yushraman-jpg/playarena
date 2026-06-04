package auth_integration_test

import (
	"context"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestRefresh_Success verifies that a valid refresh token returns 200 with a
// new access token and refresh token.
func TestRefresh_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	_, refreshToken := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	newAccess, newRefresh := apiRefresh(t, ts, refreshToken, "")

	if newAccess == "" {
		t.Error("refresh: empty access_token")
	}
	if newRefresh == "" {
		t.Error("refresh: empty refresh_token")
	}
	if newRefresh == refreshToken {
		t.Error("refresh: token was not rotated (old == new)")
	}
}

// TestRefresh_TokenRotation verifies that after a successful refresh the old
// refresh token is invalidated (Case 2 — rotated, no session wipe).
func TestRefresh_TokenRotation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	_, oldToken := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	_, _ = apiRefresh(t, ts, oldToken, "")

	// Attempt to use the old (now rotated) token — must fail with 401.
	resp := ts.post(t, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": oldToken,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
}

// TestRefresh_Case2_RotatedReplay verifies the structural Case 2 of the replay
// state machine: re-presenting a rotated token returns ErrInvalidToken (401)
// without wiping the remaining session. The legitimate holder of the successor
// token must still be able to refresh.
func TestRefresh_Case2_RotatedReplay(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	_, oldToken := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)

	// Rotation: old → successor.
	_, successorToken := apiRefresh(t, ts, oldToken, "")

	// Case 2 replay: re-present the rotated old token.
	replayResp := ts.post(t, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": oldToken,
	})
	defer replayResp.Body.Close()
	assertStatus(t, replayResp, 401) // ErrInvalidToken

	// The successor token must still be usable (no session wipe in Case 2).
	_, newToken := apiRefresh(t, ts, successorToken, "")
	if newToken == "" {
		t.Fatal("Case 2: successor token should still be valid after old-token replay")
	}
}

// TestRefresh_Case3_RevokedReplay verifies the structural Case 3 of the replay
// state machine: re-presenting an explicitly revoked token (logged-out) returns
// ErrTokenReuse (401) AND triggers a full session wipe for the user.
func TestRefresh_Case3_RevokedReplay(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Open two independent sessions.
	_, session1 := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	_, session2 := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)

	// Explicitly revoke session1 via logout.
	apiLogout(t, ts, session1)

	// Case 3 replay: present the explicitly revoked token.
	replayResp := ts.post(t, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": session1,
	})
	defer replayResp.Body.Close()
	assertStatus(t, replayResp, 401) // ErrTokenReuse — all sessions wiped.

	// session2 must now be revoked (full wipe).
	wipeResp := ts.post(t, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": session2,
	})
	defer wipeResp.Body.Close()
	assertStatus(t, wipeResp, 401)
}

// TestRefresh_ExpiredToken verifies that a refresh token past its expiry
// returns 401.
//
// The user must have an org grant so resolveOrgContext succeeds and control
// reaches RotateRefreshToken where the expiry check lives. Without an org the
// service returns 409 (ErrOrganizationRequired) before checking expiry.
func TestRefresh_ExpiredToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	rawExpired := fixtures.CreateExpiredRefreshToken(ctx, t, testPool, user.ID)

	resp := ts.post(t, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": rawExpired,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
}

// TestRefresh_InvalidToken verifies that a garbage refresh token returns 401.
func TestRefresh_InvalidToken(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": "this-is-not-a-real-token",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
}

// TestRefresh_SuspendedUserBlocked verifies that suspending a user after they
// obtain a refresh token prevents them from exchanging it.
//
// This test exercises the service-layer pre-check: service.Refresh calls
// assertUserActive(user) after GetUserByID and before entering
// RotateRefreshToken. Because suspension is applied to the DB before the
// refresh request is made, the pre-check sees the updated status and returns
// ErrUserSuspended → 403.
//
// Step 3b (the in-transaction user-status re-check inside RotateRefreshToken,
// added by Phase 13A Auth Hardening v2) is defense-in-depth for the race where
// a suspension arrives AFTER the service pre-check but BEFORE the rotation
// transaction commits. That sub-millisecond window cannot be deterministically
// exercised in a sequential integration test; it is an accepted architectural
// constraint, not a gap in this test.
func TestRefresh_SuspendedUserBlocked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	_, refreshToken := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)

	// Suspend the user in the DB after they obtained their token.
	if _, err := testPool.Exec(ctx,
		"UPDATE users SET status = 'suspended' WHERE id = $1", user.ID,
	); err != nil {
		t.Fatalf("suspend user: %v", err)
	}

	resp := ts.post(t, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": refreshToken,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "account suspended")
}

// TestRefresh_SessionsRevokedByPasswordReset verifies that after a successful
// password reset all existing refresh tokens are revoked (successor_id = NULL,
// Case 3 on next presentation).
func TestRefresh_SessionsRevokedByPasswordReset(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Open two sessions.
	_, session1 := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	_, session2 := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)

	// Reset the password — revokes all active sessions.
	rawResetToken := apiForgotPassword(t, ts, user.Email)
	apiResetPassword(t, ts, rawResetToken, "NewPassword2!")

	// Both sessions must now be invalid.
	resp1 := ts.post(t, "/api/v1/auth/refresh", map[string]string{"refresh_token": session1})
	defer resp1.Body.Close()
	assertStatus(t, resp1, 401)

	resp2 := ts.post(t, "/api/v1/auth/refresh", map[string]string{"refresh_token": session2})
	defer resp2.Body.Close()
	assertStatus(t, resp2, 401)
}
