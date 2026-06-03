package auth_integration_test

import (
	"context"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestLogin_SingleOrgAutoSelect verifies that a user belonging to exactly one
// organization can log in without providing an organization_id — the service
// auto-selects the single org and embeds it in the access token.
func TestLogin_SingleOrgAutoSelect(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Login with no organization_id — should auto-select the single org.
	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    user.Email,
		"password": fixtures.KnownPasswordRaw,
		// no organization_id
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var r loginResp
	decodeBody(t, resp, &r)
	if r.AccessToken == "" {
		t.Fatal("single-org auto-select: empty access_token")
	}

	// Verify that the me response carries the auto-selected org.
	me := apiMe(t, ts, r.AccessToken)
	if me.OrganizationID != orgID {
		t.Errorf("single-org auto-select: me.organization_id got %q, want %q",
			me.OrganizationID, orgID)
	}
}

// TestLogin_MultiOrgRequired_409 verifies that a user belonging to two or more
// organizations must provide an organization_id. Without one, the server returns
// 409 with a body containing the org list so the client can present a picker UI.
func TestLogin_MultiOrgRequired_409(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	// Grant the user roles in two separate organizations.
	orgID1 := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	orgID2 := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_admin")

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    user.Email,
		"password": fixtures.KnownPasswordRaw,
		// no organization_id
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 409)

	var r orgRequiredResp
	decodeBody(t, resp, &r)

	if r.Code != "organization_required" {
		t.Errorf("multi-org 409: code got %q, want %q", r.Code, "organization_required")
	}
	if len(r.Organizations) != 2 {
		t.Errorf("multi-org 409: org count got %d, want 2", len(r.Organizations))
	}

	// Both org IDs must be present in the list.
	found := map[string]bool{orgID1: false, orgID2: false}
	for _, o := range r.Organizations {
		found[o.ID] = true
	}
	for id, present := range found {
		if !present {
			t.Errorf("multi-org 409: org %q missing from organizations list", id)
		}
	}
}

// TestLogin_MultiOrgExplicit verifies that a multi-org user can log in by
// providing a specific organization_id, and the resulting access token carries
// that org's UUID.
func TestLogin_MultiOrgExplicit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	orgID1 := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")
	fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_admin")

	// Login with explicit org A.
	accessToken, _ := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID1)

	me := apiMe(t, ts, accessToken)
	if me.OrganizationID != orgID1 {
		t.Errorf("multi-org explicit: me.organization_id got %q, want %q",
			me.OrganizationID, orgID1)
	}
}

// TestLogin_WrongOrgID verifies that supplying an organization_id for an org
// the user does not belong to returns 422 "organization not found or access denied".
func TestLogin_WrongOrgID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Create a second user + org that the first user is NOT a member of.
	otherUser := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, otherUser.ID) })
	otherOrgID := fixtures.CreateOrgWithRole(ctx, t, testPool, otherUser.ID, "org_owner")

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":           user.Email,
		"password":        fixtures.KnownPasswordRaw,
		"organization_id": otherOrgID,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 422)
	assertErrorBody(t, resp, "organization not found or access denied")
}

// TestLogin_PlatformAdmin verifies that a user granted the platform_admin role
// (organization_id = NULL) can log in without an organization_id and receives
// an access token where organization_id is the empty string.
func TestLogin_PlatformAdmin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	admin := fixtures.CreatePlatformAdmin(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, admin.ID) })

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    admin.Email,
		"password": fixtures.KnownPasswordRaw,
		// no organization_id — platform admin auto-selects platform context
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var r loginResp
	decodeBody(t, resp, &r)
	if r.AccessToken == "" {
		t.Fatal("platform admin: empty access_token")
	}

	me := apiMe(t, ts, r.AccessToken)
	if me.OrganizationID != "" {
		t.Errorf("platform admin: me.organization_id got %q, want empty string",
			me.OrganizationID)
	}
}

// TestMultiTenant_CrossOrgRefreshDenied verifies that a user cannot use their
// refresh token to obtain an access token for an organization they are not a
// member of. The refresh endpoint performs the same org resolution as login.
func TestMultiTenant_CrossOrgRefreshDenied(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Create a different org that user has no role in.
	otherUser := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, otherUser.ID) })
	foreignOrgID := fixtures.CreateOrgWithRole(ctx, t, testPool, otherUser.ID, "org_owner")

	_, refreshToken := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)

	// Attempt to refresh with the foreign org's ID.
	resp := ts.post(t, "/api/v1/auth/refresh", map[string]any{
		"refresh_token":   refreshToken,
		"organization_id": foreignOrgID,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 422)
	assertErrorBody(t, resp, "organization not found or access denied")
}
