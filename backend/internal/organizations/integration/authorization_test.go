package organizations_integration_test

import (
	"net/http"
	"testing"
)

// TestOrg_Create_NoAuth verifies POST /api/v1/organizations without a token
// returns 401.
func TestOrg_Create_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/organizations", map[string]any{
		"name": "Unauthorized Org",
		"type": "club",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

// TestOrg_Create_NoPermission verifies a viewer-role token on POST returns 403.
func TestOrg_Create_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	viewer := setupUserAndOrg(t, ts, "viewer")

	resp := ts.postWithHeaders(t, "/api/v1/organizations", map[string]any{
		"name": "Should Not Create",
		"type": "club",
	}, bearerHeader(viewer.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestOrg_Update_NoPermission verifies a viewer-role token on PATCH returns 403.
func TestOrg_Update_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	viewer := setupUserAndOrg(t, ts, "viewer")

	resp := ts.patch(t, "/api/v1/organizations/"+actor.orgSlug, map[string]any{
		"name": "Should Not Update",
	}, bearerHeader(viewer.token))
	defer resp.Body.Close()
	// viewer doesn't have organization.update — first hits RBAC 403
	assertStatus(t, resp, http.StatusForbidden)
}

// TestOrg_Delete_NoPermission verifies a viewer-role token on DELETE returns 403.
func TestOrg_Delete_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	viewer := setupUserAndOrg(t, ts, "viewer")

	resp := ts.delete(t, "/api/v1/organizations/"+actor.orgSlug, bearerHeader(viewer.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestOrg_List_NoAuth verifies GET /api/v1/organizations without a token
// returns 401.
func TestOrg_List_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)

	resp := ts.get(t, "/api/v1/organizations", nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}
