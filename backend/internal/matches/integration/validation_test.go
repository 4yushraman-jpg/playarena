package matches_integration_test

import (
	"context"
	"net/http"
	"testing"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestMatch_Create_MissingTournamentID verifies missing tournament_id returns 400.
func TestMatch_Create_MissingTournamentID(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, matchesURL(actor.orgSlug), map[string]any{
		"scheduled_at": scheduledAt(),
	}, bearerHeader(actor.token))
	assertValidationError(t, resp, "tournament_id")
}

// TestMatch_Create_MissingScheduledAt verifies missing scheduled_at returns 400.
func TestMatch_Create_MissingScheduledAt(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, matchesURL(actor.orgSlug), map[string]any{
		"tournament_id": "00000000-0000-0000-0000-000000000001",
	}, bearerHeader(actor.token))
	assertValidationError(t, resp, "scheduled_at")
}

// TestMatch_Create_MalformedJSON verifies syntactically invalid body returns 400.
func TestMatch_Create_MalformedJSON(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postRawWithHeaders(t, matchesURL(actor.orgSlug), `{"tournament_id":}`, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestMatch_Create_TournamentNotOngoing verifies creating a match for a non-ongoing
// tournament returns 400.
func TestMatch_Create_TournamentNotOngoing(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)

	// Draft tournament — not in ongoing status.
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	homeTeamID := pgutil.UUIDToString(setup.HomeTeam.ID)
	awayTeamID := pgutil.UUIDToString(setup.AwayTeam.ID)

	// Use the ongoing tournament's org but a draft tournament.
	draftTournament := fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusDraft)
	draftTournamentID := pgutil.UUIDToString(draftTournament.ID)

	resp := ts.postWithHeaders(t, matchesURL(actor.orgSlug), map[string]any{
		"tournament_id": draftTournamentID,
		"home_team_id":  homeTeamID,
		"away_team_id":  awayTeamID,
		"scheduled_at":  scheduledAt(),
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestMatch_StatusTransition_Invalid verifies an invalid transition (scheduled→completed)
// returns 400.
func TestMatch_StatusTransition_Invalid(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.patch(t, matchURL(actor.orgSlug, matchID), map[string]any{
		"status": "completed",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}
