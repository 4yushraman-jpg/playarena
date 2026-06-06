package players_integration_test

import (
	"net/http"
	"testing"
)

// TestPlayer_Create_EmptyDisplayName verifies display_name="" returns 400
// with a validation error.
func TestPlayer_Create_EmptyDisplayName(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, playersURL(actor.orgSlug), map[string]any{
		"display_name": "",
	}, bearerHeader(actor.token))
	assertValidationError(t, resp, "display_name")
}

// TestPlayer_Create_MalformedJSON verifies syntactically invalid body returns 400.
func TestPlayer_Create_MalformedJSON(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postRawWithHeaders(t, playersURL(actor.orgSlug), `{"display_name":}`, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestPlayer_List_InvalidStatusFilter verifies ?status=garbage returns 422.
func TestPlayer_List_InvalidStatusFilter(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, playersURL(actor.orgSlug)+"?status=not_a_status", bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnprocessableEntity)
}

// TestPlayer_Get_NonexistentID verifies GET with a non-existent UUID returns 404.
func TestPlayer_Get_NonexistentID(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, playerURL(actor.orgSlug, "00000000-0000-0000-0000-000000000001"), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}
