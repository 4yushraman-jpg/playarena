package members_integration_test

import (
	"context"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestMembers_RequiresAuth verifies that all endpoints reject unauthenticated
// requests with 401.
func TestMembers_RequiresAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)

	actor := setupUserAndOrg(t, ts, "org_owner")

	endpoints := []func() int{
		func() int {
			r := ts.getWithToken(t, membersURL(actor.orgSlug), "")
			defer r.Body.Close()
			return r.StatusCode
		},
		func() int {
			r := ts.getWithToken(t, memberURL(actor.orgSlug, "00000000-0000-0000-0000-000000000001"), "")
			defer r.Body.Close()
			return r.StatusCode
		},
		func() int {
			r := ts.postWithToken(t, grantURL(actor.orgSlug, "00000000-0000-0000-0000-000000000001"), map[string]any{"role_slug": "viewer"}, "")
			defer r.Body.Close()
			return r.StatusCode
		},
		func() int {
			r := ts.deleteWithToken(t, revokeURL(actor.orgSlug, "00000000-0000-0000-0000-000000000001", "viewer"), "")
			defer r.Body.Close()
			return r.StatusCode
		},
	}

	for i, ep := range endpoints {
		if got := ep(); got != 401 {
			t.Errorf("endpoint[%d]: expected 401, got %d", i, got)
		}
	}
}

// TestMembers_RequiresPermission verifies that a viewer (who lacks role.assign)
// cannot access the members endpoints.
func TestMembers_RequiresPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	owner := setupUserAndOrg(t, ts, "org_owner")
	viewerUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	ownerOrgID, _ := pgutil.ParseUUID(owner.orgID)
	fixtures.AddUserToOrg(ctx, t, ts.pool, ownerOrgID, viewerUser.ID, "viewer")

	viewerToken := loginAs(t, ts, viewerUser.Email, fixtures.KnownPasswordRaw, owner.orgID)

	resp := ts.getWithToken(t, membersURL(owner.orgSlug), viewerToken)
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
}

// TestMembers_BOLA_WrongOrg verifies that an actor from org A cannot manage
// members of org B (403, not 404 — org exists, actor just has no access).
func TestMembers_BOLA_WrongOrg(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	// Actor scoped to org A tries to list org B's members.
	resp := ts.getWithToken(t, membersURL(orgB.orgSlug), orgA.token)
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
}

// TestMembers_GrantRole_UnknownRole verifies that granting a non-existent role
// slug returns 404.
func TestMembers_GrantRole_UnknownRole(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	actor := setupUserAndOrg(t, ts, "org_owner")
	target := fixtures.CreateActiveUser(ctx, t, ts.pool)
	targetID := pgutil.UUIDToString(target.ID)

	resp := ts.postWithToken(t, grantURL(actor.orgSlug, targetID), map[string]any{
		"role_slug": "nonexistent_role_that_does_not_exist",
	}, actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, 404)
}

// TestMembers_GrantRole_UnknownUser verifies that granting a role to a
// non-existent user returns 404.
func TestMembers_GrantRole_UnknownUser(t *testing.T) {
	ts := buildTestServer(t, testPool)

	actor := setupUserAndOrg(t, ts, "org_owner")
	nonExistentUserID := "00000000-0000-0000-0000-000000000099"

	resp := ts.postWithToken(t, grantURL(actor.orgSlug, nonExistentUserID), map[string]any{
		"role_slug": "viewer",
	}, actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, 404)
}

// TestMembers_GetMember_UnknownUser verifies that fetching a member that does
// not exist returns 404.
func TestMembers_GetMember_UnknownUser(t *testing.T) {
	ts := buildTestServer(t, testPool)

	actor := setupUserAndOrg(t, ts, "org_owner")
	nonExistentUserID := "00000000-0000-0000-0000-000000000098"

	resp := ts.getWithToken(t, memberURL(actor.orgSlug, nonExistentUserID), actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, 404)
}
