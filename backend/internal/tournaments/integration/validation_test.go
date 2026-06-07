package tournaments_integration_test

import (
	"net/http"
	"testing"
)

// TestTournament_Create_EmptyName verifies name="" returns a 400 validation error.
func TestTournament_Create_EmptyName(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, tournamentsURL(actor.orgSlug), map[string]any{
		"name":   "",
		"sport":  "kabaddi",
		"format": "league",
	}, bearerHeader(actor.token))
	assertValidationError(t, resp, "name")
}

// TestTournament_Create_MissingFormat verifies missing format returns 400.
func TestTournament_Create_MissingFormat(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, tournamentsURL(actor.orgSlug), map[string]any{
		"name":  "No Format Cup",
		"sport": "kabaddi",
	}, bearerHeader(actor.token))
	assertValidationError(t, resp, "format")
}

// TestTournament_Create_InvalidFormat verifies unrecognised format value returns 400.
func TestTournament_Create_InvalidFormat(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, tournamentsURL(actor.orgSlug), map[string]any{
		"name":   "Bad Format Cup",
		"sport":  "kabaddi",
		"format": "not_a_real_format",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestTournament_Create_InvalidCountry verifies a non-ISO country code returns 400.
func TestTournament_Create_InvalidCountry(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, tournamentsURL(actor.orgSlug), map[string]any{
		"name":    "Country Cup",
		"sport":   "kabaddi",
		"format":  "league",
		"country": "INDIA",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestTournament_Create_InvalidStatusTransition verifies advancing from draft to ongoing
// (skipping registration_open) returns 422.
func TestTournament_Create_InvalidStatusTransition(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	// Create a draft tournament, then try to jump directly to "ongoing".
	createResp := ts.postWithHeaders(t, tournamentsURL(actor.orgSlug),
		createTournamentPayload("Transition Cup"),
		bearerHeader(actor.token))
	defer createResp.Body.Close()
	assertStatus(t, createResp, http.StatusCreated)
	var trmt tournamentResponse
	decodeBody(t, createResp, &trmt)

	resp := ts.patch(t, tournamentURL(actor.orgSlug, trmt.ID), map[string]any{
		"status": "ongoing",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnprocessableEntity)
}

// TestTournament_Create_MalformedJSON verifies syntactically invalid body returns 400.
func TestTournament_Create_MalformedJSON(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postRawWithHeaders(t, tournamentsURL(actor.orgSlug), `{"name":}`, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}
