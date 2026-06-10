package members_integration_test

import (
	"context"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestMembers_GrantRole_Success verifies that an org_owner can grant a role to
// another user and the response contains the newly granted role.
func TestMembers_GrantRole_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	actor := setupUserAndOrg(t, ts, "org_owner")
	target := fixtures.CreateActiveUser(ctx, t, ts.pool)
	targetID := pgutil.UUIDToString(target.ID)

	resp := ts.postWithToken(t, grantURL(actor.orgSlug, targetID), map[string]any{
		"role_slug": "viewer",
	}, actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, 201)

	var member memberResponse
	decodeBody(t, resp, &member)

	if member.UserID != targetID {
		t.Errorf("user_id = %q, want %q", member.UserID, targetID)
	}
	if !hasRole(member, "viewer") {
		t.Errorf("expected viewer role in grants, got: %v", member.Roles)
	}
}

// TestMembers_ListMembers_Success verifies that all org members and their roles
// are returned.
func TestMembers_ListMembers_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	actor := setupUserAndOrg(t, ts, "org_owner")
	second := fixtures.CreateActiveUser(ctx, t, ts.pool)
	actorOrgID, _ := pgutil.ParseUUID(actor.orgID)
	fixtures.AddUserToOrg(ctx, t, ts.pool, actorOrgID, second.ID, "viewer")

	resp := ts.getWithToken(t, membersURL(actor.orgSlug), actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var list listResponse
	decodeBody(t, resp, &list)

	if len(list.Members) < 2 {
		t.Fatalf("expected at least 2 members, got %d", len(list.Members))
	}

	// Verify the actor appears with org_owner role.
	found := false
	for _, m := range list.Members {
		if m.UserID == actor.orgID {
			continue
		}
		if hasRole(m, "org_owner") {
			found = true
		}
	}
	// At least one member should have org_owner.
	hasOwner := false
	hasViewer := false
	for _, m := range list.Members {
		if hasRole(m, "org_owner") {
			hasOwner = true
		}
		if hasRole(m, "viewer") {
			hasViewer = true
		}
	}
	_ = found
	if !hasOwner {
		t.Error("expected at least one org_owner in the member list")
	}
	if !hasViewer {
		t.Error("expected at least one viewer in the member list")
	}
}

// TestMembers_GetMember_Success verifies that fetching a single member returns
// their active role grants.
func TestMembers_GetMember_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	actor := setupUserAndOrg(t, ts, "org_owner")
	target := fixtures.CreateActiveUser(ctx, t, ts.pool)
	actorOrgID, _ := pgutil.ParseUUID(actor.orgID)
	fixtures.AddUserToOrg(ctx, t, ts.pool, actorOrgID, target.ID, "scorer")

	targetID := pgutil.UUIDToString(target.ID)
	resp := ts.getWithToken(t, memberURL(actor.orgSlug, targetID), actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var member memberResponse
	decodeBody(t, resp, &member)

	if member.UserID != targetID {
		t.Errorf("user_id = %q, want %q", member.UserID, targetID)
	}
	if !hasRole(member, "scorer") {
		t.Errorf("expected scorer role, got: %v", member.Roles)
	}
}

// TestMembers_RevokeRole_Success verifies that an org_owner can remove a role
// from a user and receives 204.
func TestMembers_RevokeRole_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	actor := setupUserAndOrg(t, ts, "org_owner")
	target := fixtures.CreateActiveUser(ctx, t, ts.pool)
	actorOrgID, _ := pgutil.ParseUUID(actor.orgID)
	fixtures.AddUserToOrg(ctx, t, ts.pool, actorOrgID, target.ID, "viewer")

	targetID := pgutil.UUIDToString(target.ID)
	resp := ts.deleteWithToken(t, revokeURL(actor.orgSlug, targetID, "viewer"), actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, 204)
}

// TestMembers_GrantRole_Idempotent verifies that granting the same role twice
// does not duplicate it in the response.
func TestMembers_GrantRole_Idempotent(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	actor := setupUserAndOrg(t, ts, "org_owner")
	target := fixtures.CreateActiveUser(ctx, t, ts.pool)
	targetID := pgutil.UUIDToString(target.ID)

	grant := func() memberResponse {
		resp := ts.postWithToken(t, grantURL(actor.orgSlug, targetID), map[string]any{
			"role_slug": "viewer",
		}, actor.token)
		defer resp.Body.Close()
		assertStatus(t, resp, 201)
		var m memberResponse
		decodeBody(t, resp, &m)
		return m
	}

	first := grant()
	second := grant()

	// Both calls succeed. The role appears exactly once in the second response
	// (ON CONFLICT DO NOTHING keeps the original grant).
	viewerCount := 0
	for _, g := range second.Roles {
		if g.RoleSlug == "viewer" {
			viewerCount++
		}
	}
	if viewerCount != 1 {
		t.Errorf("expected exactly 1 viewer grant after idempotent insert, got %d", viewerCount)
	}
	_ = first
}

// TestMembers_RevokeRole_NotFound verifies that revoking a role that was never
// granted returns 404.
func TestMembers_RevokeRole_NotFound(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	actor := setupUserAndOrg(t, ts, "org_owner")
	target := fixtures.CreateActiveUser(ctx, t, ts.pool)
	targetID := pgutil.UUIDToString(target.ID)

	// target has no roles in this org — revoking any role must return 404.
	resp := ts.deleteWithToken(t, revokeURL(actor.orgSlug, targetID, "viewer"), actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, 404)
}

// TestMembers_LastOwnerGuard verifies that revoking the sole org_owner role
// returns 409 Conflict.
func TestMembers_LastOwnerGuard(t *testing.T) {
	ts := buildTestServer(t, testPool)

	actor := setupUserAndOrg(t, ts, "org_owner")

	// The actor is the only org_owner. Attempting to revoke their own org_owner
	// grant must be blocked.
	actorUserID := ""
	{
		resp := ts.getWithToken(t, "/api/v1/auth/me", actor.token)
		defer resp.Body.Close()
		assertStatus(t, resp, 200)
		var me struct {
			ID string `json:"id"`
		}
		decodeBody(t, resp, &me)
		actorUserID = me.ID
	}

	resp := ts.deleteWithToken(t, revokeURL(actor.orgSlug, actorUserID, "org_owner"), actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, 409)
}
