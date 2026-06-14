package fixturegen_integration_test

import (
	"net/http"
	"testing"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

// TestResolveQualifiers_GroupKnockout runs the full group+knockout flow:
// generate → play out the group stage → resolve qualifiers, and verifies the
// knockout's first round is filled with concrete participants.
func TestResolveQualifiers_GroupKnockout(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	// 8 teams, 2 groups of 4, top 2 advance → 4-qualifier knockout (2 semis + final).
	tid, _ := seedClosedTournament(t, ts, mustUUID(t, actor.orgID), db.TournamentFormatGroupKnockout, 8)

	gen := ts.postAuth(t, generateURL(actor.orgSlug, tid), map[string]any{
		"seed_strategy": "registration", "groups": 2, "qualifiers_per_group": 2,
	}, actor.token)
	defer gen.Body.Close()
	assertStatus(t, gen, http.StatusCreated)

	// Group stage: 2 groups × C(4,2)=6 = 12 matches; knockout: 3 → total 15.
	if got := countMatches(t, ts, tid); got != 15 {
		t.Fatalf("group+knockout matches = %d, want 15", got)
	}

	// Move to ongoing, play out the group stage.
	setTournamentStatus(t, ts, tid, "ongoing")
	completeAllGroupMatches(t, ts, tid)

	// Resolve qualifiers → fill the two semifinal slots (4 slots).
	res := ts.postAuth(t, resolveURL(actor.orgSlug, tid), nil, actor.token)
	defer res.Body.Close()
	assertStatus(t, res, http.StatusOK)
	var rr resolveResponse
	decodeBody(t, res, &rr)
	if rr.SlotsFilled != 4 {
		t.Fatalf("slots filled = %d, want 4", rr.SlotsFilled)
	}

	// The first knockout round (lowest knockout round_number) must now be fully
	// populated with concrete teams.
	var unresolved int
	err := ts.pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM matches
		WHERE  tournament_id = $1 AND group_label IS NULL
		  AND  round_number = (SELECT MIN(round_number) FROM matches WHERE tournament_id = $1 AND group_label IS NULL)
		  AND  (home_team_id IS NULL OR away_team_id IS NULL)`,
		mustUUID(t, tid)).Scan(&unresolved)
	if err != nil {
		t.Fatal(err)
	}
	if unresolved != 0 {
		t.Fatalf("%d first-round knockout slots unresolved after resolve", unresolved)
	}
}

// TestResolveQualifiers_GroupStageIncomplete blocks resolution mid-group-stage.
func TestResolveQualifiers_GroupStageIncomplete(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	tid, _ := seedClosedTournament(t, ts, mustUUID(t, actor.orgID), db.TournamentFormatGroupKnockout, 8)

	gen := ts.postAuth(t, generateURL(actor.orgSlug, tid), map[string]any{
		"seed_strategy": "registration", "groups": 2, "qualifiers_per_group": 2,
	}, actor.token)
	gen.Body.Close()
	assertStatus(t, gen, http.StatusCreated)
	setTournamentStatus(t, ts, tid, "ongoing")
	// Group stage NOT completed.

	res := ts.postAuth(t, resolveURL(actor.orgSlug, tid), nil, actor.token)
	defer res.Body.Close()
	assertStatus(t, res, http.StatusUnprocessableEntity)
}

// TestResolveQualifiers_NotGroupKnockout rejects resolve on a knockout tournament.
func TestResolveQualifiers_NotGroupKnockout(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	tid, _ := seedClosedTournament(t, ts, mustUUID(t, actor.orgID), db.TournamentFormatKnockout, 8)
	gen := ts.postAuth(t, generateURL(actor.orgSlug, tid), map[string]any{"seed_strategy": "registration"}, actor.token)
	gen.Body.Close()
	assertStatus(t, gen, http.StatusCreated)
	setTournamentStatus(t, ts, tid, "ongoing")

	res := ts.postAuth(t, resolveURL(actor.orgSlug, tid), nil, actor.token)
	defer res.Body.Close()
	assertStatus(t, res, http.StatusUnprocessableEntity)
}
