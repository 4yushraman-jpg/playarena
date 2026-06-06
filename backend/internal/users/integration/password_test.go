package users_integration_test

import (
	"context"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── POST /api/v1/users/{id}/change-password ────────────────────────────────

func TestChangePassword_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	resp := ts.postWithHeaders(t, "/api/v1/users/"+uuidStr(user.ID)+"/change-password", map[string]any{
		"current_password": fixtures.KnownPasswordRaw,
		"new_password":     "NewSecureP@ss1",
	}, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var m messageResp
	decodeBody(t, resp, &m)
	if m.Message == "" {
		t.Error("expected non-empty message in response")
	}
}

func TestChangePassword_WrongCurrentPassword(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	resp := ts.postWithHeaders(t, "/api/v1/users/"+uuidStr(user.ID)+"/change-password", map[string]any{
		"current_password": "wrongpassword",
		"new_password":     "NewSecureP@ss1",
	}, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
	assertErrorBody(t, resp, "current password is incorrect")
}

// TestChangePassword_AdminCannotChangeOthers verifies that even a platform
// admin cannot change another user's password. This is a self-only operation.
func TestChangePassword_AdminCannotChangeOthers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin.ID) })
	target := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, target.ID) })

	adminToken := platformAdminToken(t, admin.ID, admin.Email)

	resp := ts.postWithHeaders(t, "/api/v1/users/"+uuidStr(target.ID)+"/change-password", map[string]any{
		"current_password": fixtures.KnownPasswordRaw,
		"new_password":     "NewSecureP@ss1",
	}, bearerHeader(adminToken))
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "access denied")
}

// TestChangePassword_SessionsRevoked verifies that all pre-existing refresh
// tokens are revoked after a successful password change.
func TestChangePassword_SessionsRevoked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	// Create a live refresh token before the password change.
	_, _ = fixtures.CreateRefreshToken(ctx, t, testPool, user.ID)

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	resp := ts.postWithHeaders(t, "/api/v1/users/"+uuidStr(user.ID)+"/change-password", map[string]any{
		"current_password": fixtures.KnownPasswordRaw,
		"new_password":     "NewSecureP@ss1",
	}, bearerHeader(token))
	resp.Body.Close()
	assertStatus(t, resp, 200)

	// All refresh tokens for this user should now be revoked.
	var count int
	err := testPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL",
		user.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query active tokens: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 active refresh tokens after password change, got %d", count)
	}
}

func TestChangePassword_WeakNewPassword(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	resp := ts.postWithHeaders(t, "/api/v1/users/"+uuidStr(user.ID)+"/change-password", map[string]any{
		"current_password": fixtures.KnownPasswordRaw,
		"new_password":     "short",
	}, bearerHeader(token))
	defer resp.Body.Close()
	// Validator rejects new_password < 8 chars → 400 validation error.
	assertStatus(t, resp, 400)
}

// TestChangePassword_AuditLogged verifies that a successful password change
// writes an audit_log row with action='update' and correct entity fields (MT-15).
func TestChangePassword_AuditLogged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	resp := ts.postWithHeaders(t, "/api/v1/users/"+uuidStr(user.ID)+"/change-password", map[string]any{
		"current_password": fixtures.KnownPasswordRaw,
		"new_password":     "NewAuditedP@ss1",
	}, bearerHeader(token))
	resp.Body.Close()
	assertStatus(t, resp, 200)

	var count int
	err := testPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE action = 'update'
		  AND entity_type = 'users'
		  AND entity_id = $1
		  AND user_id = $1
		  AND old_data IS NOT NULL
		  AND new_data IS NOT NULL`,
		user.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("audit log query: %v", err)
	}
	if count != 1 {
		t.Errorf("audit_logs: got %d rows, want 1", count)
	}
}

func TestChangePassword_NoAuth(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/users/00000000-0000-0000-0000-000000000001/change-password", map[string]any{
		"current_password": "whatever",
		"new_password":     "newpassword1",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
}
