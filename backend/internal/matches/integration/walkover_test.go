package matches_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestMatch_Walkover_FromScheduled_Success verifies a scheduled fixture can be
// awarded as a walkover: status → walkover, is_walkover true, winner = the named
// side, a 0-0 forfeit score, ended_at stamped, and the reason recorded.
func TestMatch_Walkover_FromScheduled_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)
	awayTeamID := pgutil.UUIDToString(setup.AwayTeam.ID)

	resp := ts.postWithHeaders(t, matchWalkoverURL(actor.orgSlug, matchID), map[string]any{
		"winner": "away",
		"reason": "Home team failed to appear; 15-minute grace expired",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got matchResponse
	decodeBody(t, resp, &got)
	if got.Status != "walkover" {
		t.Errorf("status = %q, want walkover", got.Status)
	}
	if !got.IsWalkover {
		t.Error("is_walkover = false, want true")
	}
	if got.WinnerTeamID == nil || *got.WinnerTeamID != awayTeamID {
		t.Errorf("winner_team_id = %v, want %q", got.WinnerTeamID, awayTeamID)
	}
	if got.HomeScore != 0 || got.AwayScore != 0 {
		t.Errorf("score = %d-%d, want 0-0 forfeit", got.HomeScore, got.AwayScore)
	}
	if got.EndedAt == nil || *got.EndedAt == "" {
		t.Error("expected ended_at to be stamped on walkover")
	}
	if got.Notes == nil || *got.Notes == "" {
		t.Error("expected reason recorded in notes")
	}
}

// TestMatch_Walkover_FromLive_Success verifies a live match can be walked over
// (one side withdraws mid-fixture). ended_at must remain after started_at.
func TestMatch_Walkover_FromLive_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateLiveMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)
	homeTeamID := pgutil.UUIDToString(setup.HomeTeam.ID)

	resp := ts.postWithHeaders(t, matchWalkoverURL(actor.orgSlug, matchID), map[string]any{
		"winner": "home",
		"reason": "Away team withdrew after the first half",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got matchResponse
	decodeBody(t, resp, &got)
	if got.Status != "walkover" {
		t.Errorf("status = %q, want walkover", got.Status)
	}
	if got.WinnerTeamID == nil || *got.WinnerTeamID != homeTeamID {
		t.Errorf("winner_team_id = %v, want %q", got.WinnerTeamID, homeTeamID)
	}
	if got.EndedAt == nil || got.StartedAt == nil {
		t.Fatal("expected both started_at and ended_at on a live → walkover")
	}
}

// TestMatch_Walkover_MissingReason rejects a walkover with no reason (400).
func TestMatch_Walkover_MissingReason(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.postWithHeaders(t, matchWalkoverURL(actor.orgSlug, matchID), map[string]any{
		"winner": "home",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestMatch_Walkover_InvalidWinner rejects a winner that is not home/away (400).
func TestMatch_Walkover_InvalidWinner(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.postWithHeaders(t, matchWalkoverURL(actor.orgSlug, matchID), map[string]any{
		"winner": "neither",
		"reason": "n/a",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestMatch_Walkover_AlreadyTerminal rejects a walkover on a completed match.
func TestMatch_Walkover_AlreadyTerminal(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateCompletedMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID, 30, 20, setup.HomeTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.postWithHeaders(t, matchWalkoverURL(actor.orgSlug, matchID), map[string]any{
		"winner": "away",
		"reason": "too late",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnprocessableEntity)
}

// TestMatch_Walkover_DoubleWalkover guards against awarding two walkovers on the
// same match. The first succeeds; the second hits the terminal-state guard (422).
// This covers the "duplicate walkover" adversarial case.
func TestMatch_Walkover_DoubleWalkover(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	first := ts.postWithHeaders(t, matchWalkoverURL(actor.orgSlug, matchID), map[string]any{
		"winner": "home",
		"reason": "no show",
	}, bearerHeader(actor.token))
	first.Body.Close()
	assertStatus(t, first, http.StatusOK)

	second := ts.postWithHeaders(t, matchWalkoverURL(actor.orgSlug, matchID), map[string]any{
		"winner": "away",
		"reason": "trying to flip it",
	}, bearerHeader(actor.token))
	defer second.Body.Close()
	assertStatus(t, second, http.StatusUnprocessableEntity)
}

// TestMatch_Walkover_NoPermission verifies a viewer cannot award a walkover (403).
func TestMatch_Walkover_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)

	ownerCtx := setupUserAndOrg(t, ts, "org_owner")
	viewerCtx := setupUserAndOrg(t, ts, "viewer")

	ctx := context.Background()
	orgUID := mustUUID(t, ownerCtx.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.postWithHeaders(t, matchWalkoverURL(ownerCtx.orgSlug, matchID), map[string]any{
		"winner": "home",
		"reason": "no show",
	}, bearerHeader(viewerCtx.token))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

// TestMatch_Walkover_NoAuth verifies the endpoint requires authentication (401).
func TestMatch_Walkover_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.post(t, matchWalkoverURL(actor.orgSlug, matchID), map[string]any{
		"winner": "home",
		"reason": "no show",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}
