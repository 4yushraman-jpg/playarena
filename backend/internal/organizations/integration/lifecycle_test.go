package organizations_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestOrg_Create_Success verifies that POST /api/v1/organizations with a valid
// payload returns 201 and a body containing id, name, slug, type, and status.
func TestOrg_Create_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	adminToken := setupPlatformAdmin(t, ts)

	resp := ts.postWithHeaders(t, "/api/v1/organizations", map[string]any{
		"name": "Kabaddi Warriors",
		"type": "club",
	}, bearerHeader(adminToken))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	var body orgResponse
	decodeBody(t, resp, &body)
	if body.ID == "" {
		t.Error("expected non-empty id")
	}
	if body.Slug == "" {
		t.Error("expected non-empty slug")
	}
	if body.Type != "club" {
		t.Errorf("type: got %q, want %q", body.Type, "club")
	}
	if body.Status != "active" {
		t.Errorf("status: got %q, want %q", body.Status, "active")
	}
}

// TestOrg_Create_OnboardingUserCanCreateFirstOrg verifies the zero-org
// onboarding path: login issues an onboarding token, and that token may create
// exactly the user's first organization.
func TestOrg_Create_OnboardingUserCanCreateFirstOrg(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()
	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	token := loginAs(t, ts, user.Email, fixtures.KnownPasswordRaw, "")

	resp := ts.postWithHeaders(t, "/api/v1/organizations", map[string]any{
		"name": "First Onboarding Club",
		"type": "club",
	}, bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	var body orgResponse
	decodeBody(t, resp, &body)
	if body.ID == "" || body.Slug == "" {
		t.Fatalf("onboarding create: expected id and slug, got %#v", body)
	}

	second := ts.postWithHeaders(t, "/api/v1/organizations", map[string]any{
		"name": "Second Onboarding Club",
		"type": "club",
	}, bearerHeader(token))
	defer second.Body.Close()
	assertStatus(t, second, http.StatusForbidden)
}

// TestOrg_Create_SlugCollision verifies that two organizations with the same
// name receive unique slugs (name-2 suffix appended).
func TestOrg_Create_SlugCollision(t *testing.T) {
	ts := buildTestServer(t, testPool)
	adminToken := setupPlatformAdmin(t, ts)

	createOrg := func() orgResponse {
		resp := ts.postWithHeaders(t, "/api/v1/organizations", map[string]any{
			"name": "Slug Collision Test Org",
			"type": "club",
		}, bearerHeader(adminToken))
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusCreated)
		var body orgResponse
		decodeBody(t, resp, &body)
		return body
	}

	first := createOrg()
	second := createOrg()

	if first.Slug == second.Slug {
		t.Errorf("expected unique slugs, both got %q", first.Slug)
	}
}

// TestOrg_List_ReturnsPaginated verifies GET /api/v1/organizations returns all
// organizations with pagination metadata.
func TestOrg_List_ReturnsPaginated(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	adminToken := setupPlatformAdmin(t, ts)

	// Create two additional orgs beyond the one already created in setup.
	for range 2 {
		resp := ts.postWithHeaders(t, "/api/v1/organizations", map[string]any{
			"name": "List Test Org",
			"type": "federation",
		}, bearerHeader(adminToken))
		resp.Body.Close()
	}

	resp := ts.get(t, "/api/v1/organizations", bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list orgListResponse
	decodeBody(t, resp, &list)
	if list.Total < 1 {
		t.Errorf("total: want >= 1, got %d", list.Total)
	}
	if list.Limit <= 0 {
		t.Errorf("limit: want > 0, got %d", list.Limit)
	}
}

// TestOrg_GetBySlug_Success verifies GET /{slug} returns the correct org.
func TestOrg_GetBySlug_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, "/api/v1/organizations/"+actor.orgSlug, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var body orgResponse
	decodeBody(t, resp, &body)
	if body.ID != actor.orgID {
		t.Errorf("id: got %q, want %q", body.ID, actor.orgID)
	}
}

// TestOrg_GetBySlug_NotFound verifies GET with a non-existent slug returns 404.
func TestOrg_GetBySlug_NotFound(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, "/api/v1/organizations/this-slug-does-not-exist-xyz", bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

// TestOrg_Update_Success verifies PATCH /{slug} updates the specified fields and
// leaves the rest unchanged.
func TestOrg_Update_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	newName := "Updated Org Name"
	resp := ts.patch(t, "/api/v1/organizations/"+actor.orgSlug, map[string]any{
		"name": newName,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var body orgResponse
	decodeBody(t, resp, &body)
	if body.Name != newName {
		t.Errorf("name: got %q, want %q", body.Name, newName)
	}
	if body.ID != actor.orgID {
		t.Errorf("id should be unchanged: got %q, want %q", body.ID, actor.orgID)
	}
}

// TestOrg_Delete_Success verifies DELETE /{slug} returns 204 and the org is no
// longer accessible by slug.
func TestOrg_Delete_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.delete(t, "/api/v1/organizations/"+actor.orgSlug, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNoContent)

	// Subsequent GET should return 404.
	resp2 := ts.get(t, "/api/v1/organizations/"+actor.orgSlug, bearerHeader(actor.token))
	defer resp2.Body.Close()
	assertStatus(t, resp2, http.StatusNotFound)
}
