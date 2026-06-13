package tournaments_integration_test

import (
	"context"
	"net/http"
	"testing"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

func createTournamentPayload(name string) map[string]any {
	return map[string]any{
		"name":   name,
		"sport":  "kabaddi",
		"format": "league",
	}
}

// TestTournament_Create_Success verifies POST /tournaments returns 201 with expected fields.
func TestTournament_Create_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, tournamentsURL(actor.orgSlug),
		createTournamentPayload("Grand Kabaddi Cup"),
		bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	var trmt tournamentResponse
	decodeBody(t, resp, &trmt)
	if trmt.ID == "" {
		t.Error("expected tournament ID in response")
	}
	if trmt.Name != "Grand Kabaddi Cup" {
		t.Errorf("name = %q, want %q", trmt.Name, "Grand Kabaddi Cup")
	}
	if trmt.Status != "draft" {
		t.Errorf("status = %q, want draft", trmt.Status)
	}
	if trmt.OrganizationID != actor.orgID {
		t.Errorf("organization_id = %q, want %q", trmt.OrganizationID, actor.orgID)
	}
	if trmt.Slug == "" {
		t.Error("expected non-empty slug")
	}
}

// TestTournament_List_Default verifies GET /tournaments returns paginated list.
func TestTournament_List_Default(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusDraft)
	fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusDraft)

	resp := ts.get(t, tournamentsURL(actor.orgSlug), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list tournamentListResponse
	decodeBody(t, resp, &list)
	if list.Total < 2 {
		t.Errorf("total = %d, want >= 2", list.Total)
	}
}

// TestTournament_Get_DraftTournament verifies GET /tournaments/{id} returns a draft tournament.
func TestTournament_Get_DraftTournament(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	trmt := fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusDraft)
	trmtID := pgutil.UUIDToString(trmt.ID)

	resp := ts.get(t, tournamentURL(actor.orgSlug, trmtID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got tournamentResponse
	decodeBody(t, resp, &got)
	if got.ID != trmtID {
		t.Errorf("id = %q, want %q", got.ID, trmtID)
	}
	if got.Status != "draft" {
		t.Errorf("status = %q, want draft", got.Status)
	}
}

// TestTournament_Get_OngoingTournament verifies GET /tournaments/{id} for an ongoing tournament.
func TestTournament_Get_OngoingTournament(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	trmt := fixtures.CreateOngoingTournament(ctx, t, ts.pool, orgUID)
	trmtID := pgutil.UUIDToString(trmt.ID)

	resp := ts.get(t, tournamentURL(actor.orgSlug, trmtID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got tournamentResponse
	decodeBody(t, resp, &got)
	if got.Status != "ongoing" {
		t.Errorf("status = %q, want ongoing", got.Status)
	}
}

// TestTournament_Update_Description verifies PATCH /tournaments/{id} updates description.
func TestTournament_Update_Description(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	trmt := fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusDraft)
	trmtID := pgutil.UUIDToString(trmt.ID)

	desc := "Updated description for the tournament"
	resp := ts.patch(t, tournamentURL(actor.orgSlug, trmtID), map[string]any{
		"description": desc,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got tournamentResponse
	decodeBody(t, resp, &got)
	if got.Description == nil || *got.Description != desc {
		t.Errorf("description = %v, want %q", got.Description, desc)
	}
}

// TestTournament_Delete_SoftCancel verifies DELETE /tournaments/{id} sets status=cancelled
// but the record remains retrievable.
func TestTournament_Delete_SoftCancel(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	trmt := fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusDraft)
	trmtID := pgutil.UUIDToString(trmt.ID)

	resp := ts.delete(t, tournamentURL(actor.orgSlug, trmtID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNoContent)

	// Tournament still retrievable with cancelled status.
	getResp := ts.get(t, tournamentURL(actor.orgSlug, trmtID), bearerHeader(actor.token))
	defer getResp.Body.Close()
	assertStatus(t, getResp, http.StatusOK)
	var got tournamentResponse
	decodeBody(t, getResp, &got)
	if got.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled", got.Status)
	}
}

// TestTournament_StatusTransition_DraftToRegistrationOpen verifies the lifecycle
// transition draft→registration_open via PATCH status.
func TestTournament_StatusTransition_DraftToRegistrationOpen(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	trmt := fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusDraft)
	trmtID := pgutil.UUIDToString(trmt.ID)

	resp := ts.patch(t, tournamentURL(actor.orgSlug, trmtID), map[string]any{
		"status": "registration_open",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got tournamentResponse
	decodeBody(t, resp, &got)
	if got.Status != "registration_open" {
		t.Errorf("status = %q, want registration_open", got.Status)
	}
}

// TestTournament_Standings_EmptyOngoing verifies GET /standings returns a valid
// (possibly empty) response for an ongoing tournament.
func TestTournament_Standings_EmptyOngoing(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	trmt := fixtures.CreateOngoingTournament(ctx, t, ts.pool, orgUID)
	trmtID := pgutil.UUIDToString(trmt.ID)

	resp := ts.get(t, standingsURL(actor.orgSlug, trmtID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got standingsResponse
	decodeBody(t, resp, &got)
	if got.TournamentID != trmtID {
		t.Errorf("tournament_id = %q, want %q", got.TournamentID, trmtID)
	}
}

// TestTournament_Standings_IncludesWalkover is the FE-8A regression test for the
// standings-visibility bug: a walkover is a terminal match with a 0-0 forfeit
// result and MUST appear in standings as a win/loss, exactly like a scored
// completion. Before the ListCompletedMatchesByTournament query was widened to
// status IN ('completed','walkover'), the walkover winner showed zero wins.
func TestTournament_Standings_IncludesWalkover(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	trmtID := pgutil.UUIDToString(setup.Tournament.ID)
	homeTeamID := pgutil.UUIDToString(setup.HomeTeam.ID)
	awayTeamID := pgutil.UUIDToString(setup.AwayTeam.ID)

	// Home team awarded the win by walkover.
	fixtures.CreateWalkoverMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID, setup.HomeTeam.ID)

	resp := ts.get(t, standingsURL(actor.orgSlug, trmtID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got standingsResponse
	decodeBody(t, resp, &got)

	rows := make(map[string]standingsRowResp, len(got.Standings))
	for _, r := range got.Standings {
		rows[r.ParticipantID] = r
	}

	home, ok := rows[homeTeamID]
	if !ok {
		t.Fatalf("home team missing from standings; rows: %+v", got.Standings)
	}
	if home.Wins != 1 || home.Played != 1 || home.Points != 3 {
		t.Errorf("walkover winner standings: wins=%d played=%d points=%d, want 1/1/3 (walkover invisible to standings?)",
			home.Wins, home.Played, home.Points)
	}
	away, ok := rows[awayTeamID]
	if !ok {
		t.Fatalf("away team missing from standings; rows: %+v", got.Standings)
	}
	if away.Losses != 1 || away.Played != 1 {
		t.Errorf("walkover loser standings: losses=%d played=%d, want 1/1", away.Losses, away.Played)
	}
}

// TestTournament_Get_NotFound verifies GET with a non-existent UUID returns 404.
func TestTournament_Get_NotFound(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, tournamentURL(actor.orgSlug, "00000000-0000-0000-0000-000000000001"),
		bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}
