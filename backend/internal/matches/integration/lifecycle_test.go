package matches_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestMatch_Create_Success verifies POST /matches returns 201 with expected fields.
func TestMatch_Create_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(setup.Tournament.ID)
	homeTeamID := pgutil.UUIDToString(setup.HomeTeam.ID)
	awayTeamID := pgutil.UUIDToString(setup.AwayTeam.ID)

	resp := ts.postWithHeaders(t, matchesURL(actor.orgSlug), map[string]any{
		"tournament_id": tournamentID,
		"home_team_id":  homeTeamID,
		"away_team_id":  awayTeamID,
		"scheduled_at":  scheduledAt(),
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	var match matchResponse
	decodeBody(t, resp, &match)
	if match.ID == "" {
		t.Error("expected match ID in response")
	}
	if match.Status != "scheduled" {
		t.Errorf("status = %q, want scheduled", match.Status)
	}
	if match.HomeTeamID == nil || *match.HomeTeamID != homeTeamID {
		t.Errorf("home_team_id = %v, want %q", match.HomeTeamID, homeTeamID)
	}
	if match.AwayTeamID == nil || *match.AwayTeamID != awayTeamID {
		t.Errorf("away_team_id = %v, want %q", match.AwayTeamID, awayTeamID)
	}
}

// TestMatch_List_Default verifies GET /matches returns paginated list.
func TestMatch_List_Default(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)

	fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)

	resp := ts.get(t, matchesURL(actor.orgSlug), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list matchListResponse
	decodeBody(t, resp, &list)
	if list.Total < 2 {
		t.Errorf("total = %d, want >= 2", list.Total)
	}
}

// TestMatch_Get_ScheduledMatch verifies GET /matches/{id} returns a scheduled match.
func TestMatch_Get_ScheduledMatch(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.get(t, matchURL(actor.orgSlug, matchID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got matchResponse
	decodeBody(t, resp, &got)
	if got.ID != matchID {
		t.Errorf("id = %q, want %q", got.ID, matchID)
	}
	if got.Status != "scheduled" {
		t.Errorf("status = %q, want scheduled", got.Status)
	}
}

// TestMatch_Get_LiveMatch verifies GET /matches/{id} returns a live match.
func TestMatch_Get_LiveMatch(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateLiveMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.get(t, matchURL(actor.orgSlug, matchID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got matchResponse
	decodeBody(t, resp, &got)
	if got.Status != "live" {
		t.Errorf("status = %q, want live", got.Status)
	}
}

// TestMatch_StatusTransition_ScheduledToLive verifies PATCH transitions scheduled→live.
func TestMatch_StatusTransition_ScheduledToLive(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.patch(t, matchURL(actor.orgSlug, matchID), map[string]any{
		"status": "live",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got matchResponse
	decodeBody(t, resp, &got)
	if got.Status != "live" {
		t.Errorf("status = %q, want live", got.Status)
	}
	if got.StartedAt == nil || *got.StartedAt == "" {
		t.Error("expected started_at to be stamped on live transition")
	}
}

// TestMatch_StatusTransition_LiveToCompleted verifies PATCH transitions live→completed.
func TestMatch_StatusTransition_LiveToCompleted(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateLiveMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)
	homeTeamID := pgutil.UUIDToString(setup.HomeTeam.ID)

	resp := ts.patch(t, matchURL(actor.orgSlug, matchID), map[string]any{
		"status":         "completed",
		"winner_team_id": homeTeamID,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got matchResponse
	decodeBody(t, resp, &got)
	if got.Status != "completed" {
		t.Errorf("status = %q, want completed", got.Status)
	}
	if got.WinnerTeamID == nil || *got.WinnerTeamID != homeTeamID {
		t.Errorf("winner_team_id = %v, want %q", got.WinnerTeamID, homeTeamID)
	}
}

// TestMatch_Delete_SoftCancel verifies DELETE /matches/{id} sets status=cancelled.
func TestMatch_Delete_SoftCancel(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.delete(t, matchURL(actor.orgSlug, matchID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNoContent)

	// Match still retrievable with cancelled status.
	getResp := ts.get(t, matchURL(actor.orgSlug, matchID), bearerHeader(actor.token))
	defer getResp.Body.Close()
	assertStatus(t, getResp, http.StatusOK)
	var got matchResponse
	decodeBody(t, getResp, &got)
	if got.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled", got.Status)
	}
}

// TestMatch_GetScore_LiveMatch verifies GET /matches/{id}/score returns live score.
func TestMatch_GetScore_LiveMatch(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateLiveMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.get(t, matchScoreURL(actor.orgSlug, matchID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var score scoreResponse
	decodeBody(t, resp, &score)
	if score.MatchID != matchID {
		t.Errorf("match_id = %q, want %q", score.MatchID, matchID)
	}
}

// TestMatch_Get_NotFound verifies GET with a non-existent UUID returns 404.
func TestMatch_Get_NotFound(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, matchURL(actor.orgSlug, "00000000-0000-0000-0000-000000000001"),
		bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}
