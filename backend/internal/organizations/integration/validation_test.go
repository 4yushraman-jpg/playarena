package organizations_integration_test

import (
	"net/http"
	"testing"
)

// TestOrg_Create_EmptyName verifies an empty name returns 400 with a
// validation error on the "name" field.
func TestOrg_Create_EmptyName(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, "/api/v1/organizations", map[string]any{
		"name": "",
		"type": "club",
	}, bearerHeader(actor.token))
	assertValidationError(t, resp, "name")
}

// TestOrg_Create_InvalidType verifies an unknown org type returns 400.
func TestOrg_Create_InvalidType(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, "/api/v1/organizations", map[string]any{
		"name": "Bad Type Org",
		"type": "unknown_type",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestOrg_Create_InvalidCountry verifies a 1-character country code returns 400.
func TestOrg_Create_InvalidCountry(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	country := "X"
	resp := ts.postWithHeaders(t, "/api/v1/organizations", map[string]any{
		"name":    "Bad Country Org",
		"type":    "club",
		"country": country,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestOrg_Create_InvalidCountryLong verifies a 3-character country code returns 400.
func TestOrg_Create_InvalidCountryLong(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	country := "USA"
	resp := ts.postWithHeaders(t, "/api/v1/organizations", map[string]any{
		"name":    "Long Country Org",
		"type":    "club",
		"country": country,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestOrg_Update_InvalidType verifies PATCH with an unknown type returns 400.
func TestOrg_Update_InvalidType(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.patch(t, "/api/v1/organizations/"+actor.orgSlug, map[string]any{
		"type": "not_a_valid_type",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestOrg_Create_MalformedJSON verifies a syntactically invalid body returns 400.
func TestOrg_Create_MalformedJSON(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postRawWithHeaders(t, "/api/v1/organizations", `{"name": "broken", "type":}`, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}
