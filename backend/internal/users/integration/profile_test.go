package users_integration_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── GET /api/v1/users/{id} ─────────────────────────────────────────────────

func TestGetUser_Self(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	resp := ts.get(t, "/api/v1/users/"+uuidStr(user.ID), bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var u userResp
	decodeBody(t, resp, &u)
	if u.ID != uuidStr(user.ID) {
		t.Errorf("id: got %q, want %q", u.ID, uuidStr(user.ID))
	}
	if u.Email != user.Email {
		t.Errorf("email: got %q, want %q", u.Email, user.Email)
	}
}

func TestGetUser_OtherUser_Forbidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	actor := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, actor.ID) })
	target := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, target.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, actor.ID, "org_owner")
	token := orgUserToken(t, actor.ID, orgID, "org_owner", actor.Email)

	resp := ts.get(t, "/api/v1/users/"+uuidStr(target.ID), bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "access denied")
}

func TestGetUser_PlatformAdmin_CanAccessAny(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin.ID) })
	target := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, target.ID) })

	token := platformAdminToken(t, admin.ID, admin.Email)

	resp := ts.get(t, "/api/v1/users/"+uuidStr(target.ID), bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var u userResp
	decodeBody(t, resp, &u)
	if u.ID != uuidStr(target.ID) {
		t.Errorf("id: got %q, want %q", u.ID, uuidStr(target.ID))
	}
}

func TestGetUser_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin.ID) })

	token := platformAdminToken(t, admin.ID, admin.Email)

	resp := ts.get(t, "/api/v1/users/00000000-0000-0000-0000-000000000001", bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 404)
	assertErrorBody(t, resp, "user not found")
}

func TestGetUser_NoAuth(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.get(t, "/api/v1/users/00000000-0000-0000-0000-000000000001", nil)
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
}

// ── PATCH /api/v1/users/{id} ───────────────────────────────────────────────

func TestUpdateProfile_Self(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	resp := ts.patch(t, "/api/v1/users/"+uuidStr(user.ID), map[string]any{
		"first_name": "UpdatedFirst",
	}, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var u userResp
	decodeBody(t, resp, &u)
	if u.FullName != "UpdatedFirst User" {
		t.Errorf("full_name: got %q, want %q", u.FullName, "UpdatedFirst User")
	}
}

func TestUpdateProfile_OtherUser_Forbidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	actor := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, actor.ID) })
	target := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, target.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, actor.ID, "org_owner")
	token := orgUserToken(t, actor.ID, orgID, "org_owner", actor.Email)

	resp := ts.patch(t, "/api/v1/users/"+uuidStr(target.ID), map[string]any{
		"first_name": "Hacked",
	}, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
}

func TestUpdateProfile_PlatformAdmin_CanUpdateAny(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin.ID) })
	target := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, target.ID) })

	token := platformAdminToken(t, admin.ID, admin.Email)

	resp := ts.patch(t, "/api/v1/users/"+uuidStr(target.ID), map[string]any{
		"first_name": "AdminUpdated",
	}, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
}

func TestUpdateProfile_EmailRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	resp := ts.patch(t, "/api/v1/users/"+uuidStr(user.ID), map[string]any{
		"email": "newemail@example.com",
	}, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 422)
	assertErrorBody(t, resp, "email cannot be updated via this endpoint")
}

func TestUpdateProfile_UsernameConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user1 := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user1.ID) })
	user2 := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user2.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user2.ID, "org_owner")
	token := orgUserToken(t, user2.ID, orgID, "org_owner", user2.Email)

	// Try to claim user1's username from user2's account.
	resp := ts.patch(t, "/api/v1/users/"+uuidStr(user2.ID), map[string]any{
		"username": user1.Username,
	}, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 409)
	assertErrorBody(t, resp, "username is already taken")
}

func TestUpdateProfile_InvalidGender(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	resp := ts.patch(t, "/api/v1/users/"+uuidStr(user.ID), map[string]any{
		"gender": "attack_helicopter",
	}, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 422)

	var body badRequestResp
	decodeBody(t, resp, &body)
	if body.Field != "gender" {
		t.Errorf("field: got %q, want %q", body.Field, "gender")
	}
}

func TestUpdateProfile_ClearPhone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)
	path := "/api/v1/users/" + uuidStr(user.ID)

	// Set a phone first.
	resp := ts.patch(t, path, map[string]any{"phone": "+1234567890"}, bearerHeader(token))
	resp.Body.Close()
	assertStatus(t, resp, 200)

	// Now clear it by sending an empty string.
	resp2 := ts.patch(t, path, map[string]any{"phone": ""}, bearerHeader(token))
	defer resp2.Body.Close()
	assertStatus(t, resp2, 200)

	var u userResp
	decodeBody(t, resp2, &u)
	if u.Phone != nil {
		t.Errorf("phone: got %v, want nil (cleared)", u.Phone)
	}
}

func TestUpdateProfile_PartialPatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	// Set first_name only — last_name must remain "User" (from fixture).
	resp := ts.patch(t, "/api/v1/users/"+uuidStr(user.ID), map[string]any{
		"first_name": "OnlyFirst",
	}, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var u userResp
	decodeBody(t, resp, &u)
	wantFull := fmt.Sprintf("OnlyFirst %s", user.LastName)
	if u.FullName != wantFull {
		t.Errorf("full_name: got %q, want %q", u.FullName, wantFull)
	}
}

// TestUpdateProfile_SuspendedUser_Forbidden verifies that a suspended user
// cannot update their own profile (MT-1).
func TestUpdateProfile_SuspendedUser_Forbidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateSuspendedUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	resp := ts.patch(t, "/api/v1/users/"+uuidStr(user.ID), map[string]any{
		"first_name": "Hacked",
	}, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "access denied")
}

// TestUpdateProfile_InactiveUser_Forbidden verifies that an inactive user
// cannot update their own profile (MT-2).
func TestUpdateProfile_InactiveUser_Forbidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateInactiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	resp := ts.patch(t, "/api/v1/users/"+uuidStr(user.ID), map[string]any{
		"first_name": "Hacked",
	}, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "access denied")
}

// TestUpdateProfile_UnauthorizedWithEmailField_Forbidden verifies that an
// unauthorized actor receives 403 (not 422) when the request includes the
// email field. Auth must be checked before the email rejection (MT-5).
func TestUpdateProfile_UnauthorizedWithEmailField_Forbidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	actor := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, actor.ID) })
	target := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, target.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, actor.ID, "org_owner")
	token := orgUserToken(t, actor.ID, orgID, "org_owner", actor.Email)

	// Actor tries to update target's email — must get 403, not 422.
	resp := ts.patch(t, "/api/v1/users/"+uuidStr(target.ID), map[string]any{
		"email": "evil@example.com",
	}, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "access denied")
}

// TestUpdateProfile_AuditLogged verifies that a successful profile update
// writes an audit_log row with action='update' and correct entity fields (MT-14).
func TestUpdateProfile_AuditLogged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	resp := ts.patch(t, "/api/v1/users/"+uuidStr(user.ID), map[string]any{
		"first_name": "Audited",
	}, bearerHeader(token))
	resp.Body.Close()
	assertStatus(t, resp, 200)

	var count int
	err := testPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE action = 'update'
		  AND entity_type = 'users'
		  AND entity_id = $1
		  AND user_id = $2
		  AND old_data IS NOT NULL
		  AND new_data IS NOT NULL`,
		user.ID, user.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("audit log query: %v", err)
	}
	if count != 1 {
		t.Errorf("audit_logs: got %d rows, want 1", count)
	}
}

// TestGetUser_OrgAdmin_CannotAccessOtherUser is a BOLA guard: an org-admin
// token (non-platform) cannot read another user's profile.
func TestGetUser_OrgAdmin_CannotAccessOtherUser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	actor := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, actor.ID) })
	target := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, target.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, actor.ID, "org_admin")
	token := orgUserToken(t, actor.ID, orgID, "org_admin", actor.Email)

	resp := ts.get(t, "/api/v1/users/"+uuidStr(target.ID), bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
}

// TestUpdateProfile_ConcurrentDeactivation verifies that a profile update
// racing against a concurrent deactivation cannot write to a deactivated user.
// The FOR UPDATE lock + status re-check inside UpdateProfileTransaction is the
// authoritative guard; this test confirms the invariants hold under concurrency.
//
// Not marked parallel: relies on the deactivation outcome being observable.
func TestUpdateProfile_ConcurrentDeactivation(t *testing.T) {
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin.ID) })
	target := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, target.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, target.ID, "org_owner")
	adminToken := platformAdminToken(t, admin.ID, admin.Email)
	userToken := orgUserToken(t, target.ID, orgID, "org_owner", target.Email)

	deactivatePath := "/api/v1/users/" + uuidStr(target.ID) + "/deactivate"
	profilePath := "/api/v1/users/" + uuidStr(target.ID)

	ready := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	var deactivateCode int
	var profileCode int

	go func() {
		defer wg.Done()
		<-ready
		deactivateCode, _, _ = doPostWithHeaders(ts, deactivatePath, nil, bearerHeader(adminToken))
	}()
	go func() {
		defer wg.Done()
		<-ready
		profileCode, _ = doPatchWithHeaders(ts, profilePath, map[string]any{"first_name": "Raced"}, bearerHeader(userToken))
	}()

	close(ready)
	wg.Wait()

	// Deactivation must always succeed.
	if deactivateCode != 204 {
		t.Fatalf("deactivation: expected 204, got %d", deactivateCode)
	}

	// Profile update either ran before deactivation (200) or was blocked by the
	// transaction-level guard (403). Any other code is unexpected.
	if profileCode != 200 && profileCode != 403 {
		t.Errorf("profile update: expected 200 or 403, got %d", profileCode)
	}

	// Regardless of ordering, the user must be inactive after deactivation committed.
	var status string
	if err := testPool.QueryRow(ctx,
		"SELECT status FROM users WHERE id = $1", target.ID,
	).Scan(&status); err != nil {
		t.Fatalf("query user status: %v", err)
	}
	if status != "inactive" {
		t.Errorf("user status after deactivation: got %q, want inactive", status)
	}
}
