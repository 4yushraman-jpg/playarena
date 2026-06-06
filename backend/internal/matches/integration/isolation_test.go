package matches_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestMatch_Create_WrongOrg_BOLA verifies an actor from Org A cannot create a
// match under Org B's URL.
func TestMatch_Create_WrongOrg_BOLA(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgAUID := mustUUID(t, orgA.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgAUID)
	tournamentID := pgutil.UUIDToString(setup.Tournament.ID)
	homeTeamID := pgutil.UUIDToString(setup.HomeTeam.ID)
	awayTeamID := pgutil.UUIDToString(setup.AwayTeam.ID)

	// Org A actor tries to create match using Org B's URL.
	resp := ts.postWithHeaders(t, matchesURL(orgB.orgSlug), map[string]any{
		"tournament_id": tournamentID,
		"home_team_id":  homeTeamID,
		"away_team_id":  awayTeamID,
		"scheduled_at":  scheduledAt(),
	}, bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestMatch_Get_WrongOrg_Invisible verifies a match from Org A is not accessible
// via Org B's URL.
func TestMatch_Get_WrongOrg_Invisible(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgAUID := mustUUID(t, orgA.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgAUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgAUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.get(t, matchURL(orgB.orgSlug, matchID), bearerHeader(orgB.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

// TestMatch_List_OrgScoped verifies GET /matches for Org A does not return matches
// belonging to Org B.
func TestMatch_List_OrgScoped(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgBUID := mustUUID(t, orgB.orgID)
	setupB := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgBUID)
	matchB := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgBUID,
		setupB.Tournament.ID, setupB.HomeTeam.ID, setupB.AwayTeam.ID)
	matchBID := pgutil.UUIDToString(matchB.ID)

	resp := ts.get(t, matchesURL(orgA.orgSlug), bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list matchListResponse
	decodeBody(t, resp, &list)
	for _, m := range list.Matches {
		if m.ID == matchBID {
			t.Errorf("Org B match %q appeared in Org A's match list", matchBID)
		}
	}
}
