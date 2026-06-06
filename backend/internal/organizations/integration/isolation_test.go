package organizations_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestOrg_Update_WrongOrg_BOLA verifies that an org_owner from Org A cannot
// PATCH Org B — the BOLA guard returns 403.
func TestOrg_Update_WrongOrg_BOLA(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	// Actor from Org A tries to update Org B.
	resp := ts.patch(t, "/api/v1/organizations/"+orgB.orgSlug, map[string]any{
		"name": "BOLA attack attempt",
	}, bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)

	// Verify Org B's name is unchanged.
	check := ts.get(t, "/api/v1/organizations/"+orgB.orgSlug, bearerHeader(orgB.token))
	defer check.Body.Close()
	assertStatus(t, check, http.StatusOK)
	var body orgResponse
	decodeBody(t, check, &body)
	if body.Name == "BOLA attack attempt" {
		t.Error("BOLA guard failed: org name was mutated")
	}
}

// TestOrg_Delete_WrongOrg_BOLA verifies that an org_owner from Org A cannot
// DELETE Org B.
func TestOrg_Delete_WrongOrg_BOLA(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.delete(t, "/api/v1/organizations/"+orgB.orgSlug, bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)

	// Verify Org B still exists.
	check := ts.get(t, "/api/v1/organizations/"+orgB.orgSlug, bearerHeader(orgB.token))
	defer check.Body.Close()
	assertStatus(t, check, http.StatusOK)
}

// TestOrg_PlatformAdmin_CanUpdateAny verifies that a platform admin can update
// any organization regardless of org context.
func TestOrg_PlatformAdmin_CanUpdateAny(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgB := setupUserAndOrg(t, ts, "org_owner")

	// Create a platform admin user and log in (no org context).
	ctx := context.Background()
	admin := fixtures.CreatePlatformAdmin(ctx, t, ts.pool)
	adminToken := loginAs(t, ts, admin.Email, fixtures.KnownPasswordRaw, "")

	resp := ts.patch(t, "/api/v1/organizations/"+orgB.orgSlug, map[string]any{
		"city": "Mumbai",
	}, bearerHeader(adminToken))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
}

// TestOrg_List_AllOrgsVisible verifies that authenticated users can see all
// organizations (the list endpoint is not org-scoped).
func TestOrg_List_AllOrgsVisible(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, "/api/v1/organizations", bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list orgListResponse
	decodeBody(t, resp, &list)

	// Both orgs should appear in the list.
	slugs := make(map[string]bool)
	for _, o := range list.Organizations {
		slugs[o.Slug] = true
	}
	if !slugs[orgA.orgSlug] {
		t.Errorf("org A slug %q not found in list", orgA.orgSlug)
	}
	if !slugs[orgB.orgSlug] {
		t.Errorf("org B slug %q not found in list", orgB.orgSlug)
	}
}
