package users_integration_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── GET /api/v1/users ─────────────────────────────────────────────────────

func TestListUsers_PlatformAdmin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin.ID) })

	token := platformAdminToken(t, admin.ID, admin.Email)

	resp := ts.get(t, "/api/v1/users", bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var list listResp
	decodeBody(t, resp, &list)
	if list.Total <= 0 {
		t.Errorf("total: got %d, want > 0", list.Total)
	}
	if list.Limit <= 0 {
		t.Errorf("limit: got %d, want > 0", list.Limit)
	}
}

func TestListUsers_RegularUser_Forbidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	token := orgUserToken(t, user.ID, orgID, "org_owner", user.Email)

	resp := ts.get(t, "/api/v1/users", bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "access denied")
}

func TestListUsers_NoAuth(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.get(t, "/api/v1/users", nil)
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
}

func TestListUsers_Pagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin.ID) })

	token := platformAdminToken(t, admin.ID, admin.Email)

	resp := ts.get(t, "/api/v1/users?limit=2&offset=0", bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var list listResp
	decodeBody(t, resp, &list)
	if list.Limit != 2 {
		t.Errorf("limit: got %d, want 2", list.Limit)
	}
	if list.Offset != 0 {
		t.Errorf("offset: got %d, want 0", list.Offset)
	}
}

// ── POST /api/v1/users/{id}/deactivate ───────────────────────────────────

func TestDeactivateUser_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	// Two admins: one acts, one is the target. Prevents last-admin guard from firing.
	admin1 := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin1.ID) })
	target := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, target.ID) })

	token := platformAdminToken(t, admin1.ID, admin1.Email)

	resp := ts.postWithHeaders(t, "/api/v1/users/"+uuidStr(target.ID)+"/deactivate", nil, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 204)
}

func TestDeactivateUser_AlreadyInactive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin.ID) })
	target := fixtures.CreateInactiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, target.ID) })

	token := platformAdminToken(t, admin.ID, admin.Email)

	resp := ts.postWithHeaders(t, "/api/v1/users/"+uuidStr(target.ID)+"/deactivate", nil, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 422)
	assertErrorBody(t, resp, "user account is already inactive")
}

func TestDeactivateUser_RegularUser_Forbidden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	actor := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, actor.ID) })
	target := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, target.ID) })

	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, actor.ID, "org_owner")
	token := orgUserToken(t, actor.ID, orgID, "org_owner", actor.Email)

	resp := ts.postWithHeaders(t, "/api/v1/users/"+uuidStr(target.ID)+"/deactivate", nil, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "access denied")
}

func TestDeactivateUser_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin.ID) })

	token := platformAdminToken(t, admin.ID, admin.Email)

	resp := ts.postWithHeaders(t, "/api/v1/users/00000000-0000-0000-0000-000000000001/deactivate", nil, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 404)
	assertErrorBody(t, resp, "user not found")
}

// TestDeactivateUser_SessionsRevoked verifies that the target user's refresh
// tokens are all revoked after successful deactivation.
func TestDeactivateUser_SessionsRevoked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin.ID) })
	target := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, target.ID) })

	fixtures.CreateRefreshToken(ctx, t, testPool, target.ID)

	token := platformAdminToken(t, admin.ID, admin.Email)
	resp := ts.postWithHeaders(t, "/api/v1/users/"+uuidStr(target.ID)+"/deactivate", nil, bearerHeader(token))
	resp.Body.Close()
	assertStatus(t, resp, 204)

	var count int
	if err := testPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL",
		target.ID,
	).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 active refresh tokens after deactivation, got %d", count)
	}
}

// ── Last-platform-admin guard ─────────────────────────────────────────────

// TestDeactivateUser_LastPlatformAdmin verifies that the sole remaining
// platform admin cannot be deactivated (sentinel lock guard).
//
// Not marked parallel: this test temporarily deactivates all other active
// platform admins to guarantee an isolated count of exactly 1.
func TestDeactivateUser_LastPlatformAdmin(t *testing.T) {
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin.ID) })

	// Deactivate every other active platform admin so only admin remains.
	isolatePlatformAdmins(ctx, t, admin.ID)

	token := platformAdminToken(t, admin.ID, admin.Email)

	resp := ts.postWithHeaders(t, "/api/v1/users/"+uuidStr(admin.ID)+"/deactivate", nil, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 409)
	assertErrorBody(t, resp, "cannot deactivate the last platform administrator")
}

// TestDeactivateUser_NonLastPlatformAdmin verifies that one of two platform
// admins can be deactivated without triggering the last-admin guard.
//
// Not marked parallel for the same reason as TestDeactivateUser_LastPlatformAdmin.
func TestDeactivateUser_NonLastPlatformAdmin(t *testing.T) {
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin1 := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin1.ID) })
	admin2 := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin2.ID) })

	// Isolate: only admin1 and admin2 are active platform admins.
	isolatePlatformAdmins(ctx, t, admin1.ID, admin2.ID)

	token := platformAdminToken(t, admin1.ID, admin1.Email)

	// Deactivate admin2 — admin1 still exists, so guard must not fire.
	resp := ts.postWithHeaders(t, "/api/v1/users/"+uuidStr(admin2.ID)+"/deactivate", nil, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 204)
}

// TestDeactivate_ConcurrentLastTwoAdmins verifies that when two goroutines
// simultaneously attempt to deactivate the last two platform admins, exactly
// one succeeds (204) and exactly one is rejected (409) (MT-6).
//
// Not marked parallel: relies on a controlled platform-admin count.
func TestDeactivate_ConcurrentLastTwoAdmins(t *testing.T) {
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin1 := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin1.ID) })
	admin2 := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin2.ID) })

	// Ensure exactly these two admins are active.
	isolatePlatformAdmins(ctx, t, admin1.ID, admin2.ID)

	tok1 := platformAdminToken(t, admin1.ID, admin1.Email)
	tok2 := platformAdminToken(t, admin2.ID, admin2.Email)

	path1 := "/api/v1/users/" + uuidStr(admin2.ID) + "/deactivate"
	path2 := "/api/v1/users/" + uuidStr(admin1.ID) + "/deactivate"

	// Barrier ensures both goroutines fire their requests simultaneously.
	ready := make(chan struct{})

	type concResult struct {
		code int
		body string
	}
	results := make([]concResult, 2)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		<-ready
		results[0].code, results[0].body, _ = doPostWithHeaders(ts, path1, nil, bearerHeader(tok1))
	}()
	go func() {
		defer wg.Done()
		<-ready
		results[1].code, results[1].body, _ = doPostWithHeaders(ts, path2, nil, bearerHeader(tok2))
	}()

	close(ready)
	wg.Wait()

	const wantMsg = "cannot deactivate the last platform administrator"
	got204, got409 := 0, 0
	for _, r := range results {
		switch r.code {
		case 204:
			got204++
		case 409:
			got409++
			var e errResp
			if jerr := json.Unmarshal([]byte(r.body), &e); jerr != nil {
				t.Errorf("409 body decode: %v", jerr)
			} else if e.Error != wantMsg {
				t.Errorf("409 error body: got %q, want %q", e.Error, wantMsg)
			}
		}
	}
	if got204 != 1 || got409 != 1 {
		t.Errorf("concurrent deactivation: got statuses [%d, %d], want exactly one 204 and one 409",
			results[0].code, results[1].code)
	}
}

// TestDeactivateUser_AuditLogged verifies that a successful deactivation writes
// an audit_log row with action='update' and correct entity fields (MT-16).
func TestDeactivateUser_AuditLogged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin.ID) })
	target := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, target.ID) })

	token := platformAdminToken(t, admin.ID, admin.Email)

	resp := ts.postWithHeaders(t, "/api/v1/users/"+uuidStr(target.ID)+"/deactivate", nil, bearerHeader(token))
	resp.Body.Close()
	assertStatus(t, resp, 204)

	var count int
	err := testPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs
		WHERE action = 'update'
		  AND entity_type = 'users'
		  AND entity_id = $1
		  AND user_id = $2
		  AND old_data IS NOT NULL
		  AND new_data IS NOT NULL`,
		target.ID, admin.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("audit log query: %v", err)
	}
	if count != 1 {
		t.Errorf("audit_logs: got %d rows, want 1", count)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// isolatePlatformAdmins deactivates all active platform admins whose IDs are
// NOT in keepIDs, and registers a t.Cleanup to reactivate them. Call this at
// the start of any non-parallel test that needs a controlled platform-admin count.
func isolatePlatformAdmins(ctx context.Context, t *testing.T, keepIDs ...pgtype.UUID) {
	t.Helper()

	// Build a list of UUIDs to exclude from deactivation.
	keepSet := make(map[string]bool, len(keepIDs))
	for _, id := range keepIDs {
		keepSet[uuidStr(id)] = true
	}

	// Find all active platform admins not in keepSet.
	rows, err := testPool.Query(ctx, `
		SELECT DISTINCT u.id FROM users u
		JOIN user_organization_roles uor ON uor.user_id = u.id
		JOIN roles r ON r.id = uor.role_id
		WHERE r.slug = 'platform_admin' AND r.scope = 'platform'
		  AND uor.organization_id IS NULL
		  AND u.status = 'active'
		  AND (uor.expires_at IS NULL OR uor.expires_at > NOW())`)
	if err != nil {
		t.Fatalf("isolatePlatformAdmins: query: %v", err)
	}
	defer rows.Close()

	var toDeactivate []pgtype.UUID
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("isolatePlatformAdmins: scan: %v", err)
		}
		if !keepSet[uuidStr(id)] {
			toDeactivate = append(toDeactivate, id)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("isolatePlatformAdmins: rows: %v", err)
	}

	for _, id := range toDeactivate {
		if _, err := testPool.Exec(ctx,
			"UPDATE users SET status = 'inactive' WHERE id = $1", id,
		); err != nil {
			t.Fatalf("isolatePlatformAdmins: deactivate %v: %v", id, err)
		}
	}

	t.Cleanup(func() {
		for _, id := range toDeactivate {
			if _, err := testPool.Exec(context.Background(),
				"UPDATE users SET status = 'active' WHERE id = $1", id,
			); err != nil {
				t.Logf("isolatePlatformAdmins cleanup: reactivate %v: %v", id, err)
			}
		}
	})
}
