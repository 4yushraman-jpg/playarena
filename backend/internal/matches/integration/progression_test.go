package matches_integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── helpers ────────────────────────────────────────────────────────────────────

// createMatchViaAPI POSTs a match and returns the decoded response. body is the
// raw create payload so tests can include/omit participants and linkage freely.
func createMatchViaAPI(t *testing.T, ts *testServer, actor orgContext, body map[string]any) matchResponse {
	t.Helper()
	resp := ts.postWithHeaders(t, matchesURL(actor.orgSlug), body, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)
	var m matchResponse
	decodeBody(t, resp, &m)
	return m
}

// scoreTeamPoint inserts a raid_successful event attributing `points` to a team,
// so a subsequent completion derives a real winner from the event log. Inserted
// directly via SQL because the match_events fixture does not set team_id/payload.
func scoreTeamPoint(t *testing.T, pool *pgxpool.Pool, orgID, matchID, teamID pgtype.UUID, points int) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO match_events (match_id, organization_id, sequence_number, event_type, team_id, payload, recorded_at)
		VALUES ($1, $2,
		        COALESCE((SELECT MAX(sequence_number) FROM match_events WHERE match_id = $1), 0) + 1,
		        'raid_successful', $3, $4::jsonb, NOW())`,
		matchID, orgID, teamID, fmt.Sprintf(`{"points":%d}`, points),
	)
	if err != nil {
		t.Fatalf("scoreTeamPoint: %v", err)
	}
}

func getMatch(t *testing.T, ts *testServer, actor orgContext, matchID string) matchResponse {
	t.Helper()
	resp := ts.get(t, matchURL(actor.orgSlug, matchID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var m matchResponse
	decodeBody(t, resp, &m)
	return m
}

// ── TBD / linkage creation ──────────────────────────────────────────────────────

// TestMatch_Create_TBDMatch verifies a match can be created with no participants
// (a downstream bracket slot awaiting propagation).
func TestMatch_Create_TBDMatch(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)

	m := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": pgutil.UUIDToString(setup.Tournament.ID),
		"scheduled_at":  scheduledAt(),
		"round_name":    "Final",
	})
	if m.Status != "scheduled" {
		t.Errorf("status = %q, want scheduled", m.Status)
	}
	if m.HomeTeamID != nil || m.AwayTeamID != nil {
		t.Errorf("expected TBD (nil participants), got home=%v away=%v", m.HomeTeamID, m.AwayTeamID)
	}
}

// TestMatch_Create_WithLinkage verifies a feeder can be linked to a successor slot.
func TestMatch_Create_WithLinkage(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	tid := pgutil.UUIDToString(setup.Tournament.ID)

	successor := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": tid,
		"scheduled_at":  scheduledAt(),
		"round_name":    "Final",
	})

	feeder := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id":   tid,
		"home_team_id":    pgutil.UUIDToString(setup.HomeTeam.ID),
		"away_team_id":    pgutil.UUIDToString(setup.AwayTeam.ID),
		"scheduled_at":    scheduledAt(),
		"next_match_id":   successor.ID,
		"next_match_slot": 1,
	})
	if feeder.NextMatchID == nil || *feeder.NextMatchID != successor.ID {
		t.Errorf("next_match_id = %v, want %q", feeder.NextMatchID, successor.ID)
	}
	if feeder.NextMatchSlot == nil || *feeder.NextMatchSlot != 1 {
		t.Errorf("next_match_slot = %v, want 1", feeder.NextMatchSlot)
	}
}

// TestMatch_Create_LinkageIncomplete rejects next_match_id without a slot (400).
func TestMatch_Create_LinkageIncomplete(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	tid := pgutil.UUIDToString(setup.Tournament.ID)
	successor := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": tid, "scheduled_at": scheduledAt(), "round_name": "Final",
	})

	resp := ts.postWithHeaders(t, matchesURL(actor.orgSlug), map[string]any{
		"tournament_id": tid,
		"home_team_id":  pgutil.UUIDToString(setup.HomeTeam.ID),
		"away_team_id":  pgutil.UUIDToString(setup.AwayTeam.ID),
		"scheduled_at":  scheduledAt(),
		"next_match_id": successor.ID, // no next_match_slot
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestMatch_Create_LinkageCrossTournament rejects linking to a successor in a
// different tournament (I5 integrity guard, 422).
func TestMatch_Create_LinkageCrossTournament(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	t1 := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	t2 := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	// A successor that lives in tournament 2.
	foreign := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID, t2.Tournament.ID, t2.HomeTeam.ID, t2.AwayTeam.ID)

	resp := ts.postWithHeaders(t, matchesURL(actor.orgSlug), map[string]any{
		"tournament_id":   pgutil.UUIDToString(t1.Tournament.ID),
		"home_team_id":    pgutil.UUIDToString(t1.HomeTeam.ID),
		"away_team_id":    pgutil.UUIDToString(t1.AwayTeam.ID),
		"scheduled_at":    scheduledAt(),
		"next_match_id":   pgutil.UUIDToString(foreign.ID),
		"next_match_slot": 1,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnprocessableEntity)
}

// TestMatch_Create_LinkageSlotCollision rejects a second feeder into a slot that
// is already fed — it would overwrite the first feeder's propagated winner (422).
func TestMatch_Create_LinkageSlotCollision(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	tid := pgutil.UUIDToString(setup.Tournament.ID)
	homeTeamID := pgutil.UUIDToString(setup.HomeTeam.ID)
	awayTeamID := pgutil.UUIDToString(setup.AwayTeam.ID)

	successor := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": tid, "scheduled_at": scheduledAt(), "round_name": "Final",
	})
	// First feeder claims the home slot.
	createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": tid, "home_team_id": homeTeamID, "away_team_id": awayTeamID,
		"scheduled_at": scheduledAt(), "next_match_id": successor.ID, "next_match_slot": 1,
	})
	// Second feeder tries to claim the same slot → rejected.
	resp := ts.postWithHeaders(t, matchesURL(actor.orgSlug), map[string]any{
		"tournament_id": tid, "home_team_id": homeTeamID, "away_team_id": awayTeamID,
		"scheduled_at": scheduledAt(), "next_match_id": successor.ID, "next_match_slot": 1,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnprocessableEntity)
}

// ── I1 guard: can't start/conclude a TBD match ──────────────────────────────────

// TestMatch_StartTBDMatch_Blocked verifies a TBD match cannot go live (I1).
func TestMatch_StartTBDMatch_Blocked(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	tbd := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": pgutil.UUIDToString(setup.Tournament.ID),
		"scheduled_at":  scheduledAt(), "round_name": "Final",
	})

	resp := ts.patch(t, matchURL(actor.orgSlug, tbd.ID), map[string]any{
		"status": "live",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnprocessableEntity)
}

// TestMatch_WalkoverTBDMatch_Blocked verifies a TBD match cannot be walked over.
func TestMatch_WalkoverTBDMatch_Blocked(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	tbd := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": pgutil.UUIDToString(setup.Tournament.ID),
		"scheduled_at":  scheduledAt(), "round_name": "Final",
	})

	resp := ts.postWithHeaders(t, matchWalkoverURL(actor.orgSlug, tbd.ID), map[string]any{
		"winner": "home", "reason": "no participants",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnprocessableEntity)
}

// ── propagation ─────────────────────────────────────────────────────────────────

// TestMatch_Propagation_WalkoverAdvancesWinner verifies a walkover winner is
// written into the linked successor's slot, in the same transaction.
func TestMatch_Propagation_WalkoverAdvancesWinner(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	tid := pgutil.UUIDToString(setup.Tournament.ID)
	homeTeamID := pgutil.UUIDToString(setup.HomeTeam.ID)

	successor := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": tid, "scheduled_at": scheduledAt(), "round_name": "Final",
	})
	feeder := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id":   tid,
		"home_team_id":    homeTeamID,
		"away_team_id":    pgutil.UUIDToString(setup.AwayTeam.ID),
		"scheduled_at":    scheduledAt(),
		"next_match_id":   successor.ID,
		"next_match_slot": 1, // home slot
	})

	resp := ts.postWithHeaders(t, matchWalkoverURL(actor.orgSlug, feeder.ID), map[string]any{
		"winner": "home", "reason": "away no-show",
	}, bearerHeader(actor.token))
	resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	// Successor's home slot now holds the feeder winner; it stays scheduled.
	got := getMatch(t, ts, actor, successor.ID)
	if got.HomeTeamID == nil || *got.HomeTeamID != homeTeamID {
		t.Errorf("successor home_team_id = %v, want %q (propagation)", got.HomeTeamID, homeTeamID)
	}
	if got.Status != "scheduled" {
		t.Errorf("successor status = %q, want scheduled", got.Status)
	}
}

// TestMatch_Propagation_CompletionAdvancesWinner verifies a scored completion
// propagates its winner identically to a walkover.
func TestMatch_Propagation_CompletionAdvancesWinner(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	tid := pgutil.UUIDToString(setup.Tournament.ID)
	homeTeamID := pgutil.UUIDToString(setup.HomeTeam.ID)

	successor := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": tid, "scheduled_at": scheduledAt(), "round_name": "Final",
	})
	feeder := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id":   tid,
		"home_team_id":    homeTeamID,
		"away_team_id":    pgutil.UUIDToString(setup.AwayTeam.ID),
		"scheduled_at":    scheduledAt(),
		"next_match_id":   successor.ID,
		"next_match_slot": 2, // away slot
	})

	// Go live, score the home team, then complete with home as the declared winner.
	live := ts.patch(t, matchURL(actor.orgSlug, feeder.ID), map[string]any{"status": "live"}, bearerHeader(actor.token))
	live.Body.Close()
	assertStatus(t, live, http.StatusOK)

	scoreTeamPoint(t, ts.pool, orgUID, mustUUID(t, feeder.ID), mustUUID(t, homeTeamID), 5)

	done := ts.patch(t, matchURL(actor.orgSlug, feeder.ID), map[string]any{
		"status":         "completed",
		"winner_team_id": homeTeamID,
	}, bearerHeader(actor.token))
	done.Body.Close()
	assertStatus(t, done, http.StatusOK)

	// Winner advanced into the successor's AWAY slot (next_match_slot = 2).
	got := getMatch(t, ts, actor, successor.ID)
	if got.AwayTeamID == nil || *got.AwayTeamID != homeTeamID {
		t.Errorf("successor away_team_id = %v, want %q (completion propagation)", got.AwayTeamID, homeTeamID)
	}
}

// TestMatch_Propagation_TwoFeedersFillBothSlots verifies two feeders advancing
// into the same successor each fill their own fixed slot, after which the
// successor is playable. Covers idempotent per-slot writes (no double propagation).
func TestMatch_Propagation_TwoFeedersFillBothSlots(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	tid := pgutil.UUIDToString(setup.Tournament.ID)
	homeTeamID := pgutil.UUIDToString(setup.HomeTeam.ID)
	awayTeamID := pgutil.UUIDToString(setup.AwayTeam.ID)

	successor := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": tid, "scheduled_at": scheduledAt(), "round_name": "Final",
	})
	// Two feeders (re-using the two approved teams) → successor home and away.
	f1 := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": tid, "home_team_id": homeTeamID, "away_team_id": awayTeamID,
		"scheduled_at": scheduledAt(), "next_match_id": successor.ID, "next_match_slot": 1,
	})
	f2 := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": tid, "home_team_id": homeTeamID, "away_team_id": awayTeamID,
		"scheduled_at": scheduledAt(), "next_match_id": successor.ID, "next_match_slot": 2,
	})

	wo := func(id, winner string) {
		r := ts.postWithHeaders(t, matchWalkoverURL(actor.orgSlug, id), map[string]any{
			"winner": winner, "reason": "x",
		}, bearerHeader(actor.token))
		r.Body.Close()
		assertStatus(t, r, http.StatusOK)
	}
	wo(f1.ID, "home") // f1 → successor.home = homeTeam
	wo(f2.ID, "away") // f2 → successor.away = awayTeam

	got := getMatch(t, ts, actor, successor.ID)
	if got.HomeTeamID == nil || *got.HomeTeamID != homeTeamID {
		t.Errorf("successor home = %v, want %q", got.HomeTeamID, homeTeamID)
	}
	if got.AwayTeamID == nil || *got.AwayTeamID != awayTeamID {
		t.Errorf("successor away = %v, want %q", got.AwayTeamID, awayTeamID)
	}
	// Now fully populated, the successor can start.
	live := ts.patch(t, matchURL(actor.orgSlug, successor.ID), map[string]any{"status": "live"}, bearerHeader(actor.token))
	live.Body.Close()
	assertStatus(t, live, http.StatusOK)
}

// TestMatch_Propagation_BlockedWhenDownstreamNotScheduled is the I3 / stale-
// propagation guard: a winner cannot flow into a successor that has already
// started, and the triggering completion is rolled back atomically.
func TestMatch_Propagation_BlockedWhenDownstreamNotScheduled(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	tid := pgutil.UUIDToString(setup.Tournament.ID)
	homeTeamID := pgutil.UUIDToString(setup.HomeTeam.ID)
	awayTeamID := pgutil.UUIDToString(setup.AwayTeam.ID)

	// Successor created already populated, then started — no longer scheduled.
	successor := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": tid, "home_team_id": homeTeamID, "away_team_id": awayTeamID,
		"scheduled_at": scheduledAt(),
	})
	startSucc := ts.patch(t, matchURL(actor.orgSlug, successor.ID), map[string]any{"status": "live"}, bearerHeader(actor.token))
	startSucc.Body.Close()
	assertStatus(t, startSucc, http.StatusOK)

	// Feeder pointing at the now-live successor.
	feeder := createMatchViaAPI(t, ts, actor, map[string]any{
		"tournament_id": tid, "home_team_id": homeTeamID, "away_team_id": awayTeamID,
		"scheduled_at": scheduledAt(), "next_match_id": successor.ID, "next_match_slot": 1,
	})

	// Walkover the feeder → propagation must block; completion rolled back.
	resp := ts.postWithHeaders(t, matchWalkoverURL(actor.orgSlug, feeder.ID), map[string]any{
		"winner": "home", "reason": "away no-show",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	// Feeder must remain scheduled (the walkover was rolled back atomically).
	got := getMatch(t, ts, actor, feeder.ID)
	if got.Status != "scheduled" {
		t.Errorf("feeder status = %q, want scheduled (walkover should have rolled back)", got.Status)
	}
}
