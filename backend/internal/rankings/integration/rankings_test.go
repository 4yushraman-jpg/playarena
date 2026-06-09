package rankings_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── unauthenticated ───────────────────────────────────────────────────────────

func TestRankings_Players_Unauthenticated(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, playerRankingsURL(actor.orgSlug), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

func TestRankings_Teams_Unauthenticated(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, teamRankingsURL(actor.orgSlug), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

// ── org not found ─────────────────────────────────────────────────────────────

func TestRankings_Players_OrgNotFound(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, playerRankingsURL("no-such-org-ever"), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

func TestRankings_Teams_OrgNotFound(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, teamRankingsURL("no-such-org-ever"), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

// ── empty lists ───────────────────────────────────────────────────────────────

func TestRankings_Players_Empty(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, playerRankingsURL(actor.orgSlug), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var body playerRankingsResponse
	decodeBody(t, resp, &body)
	if body.Total != 0 {
		t.Errorf("total = %d, want 0", body.Total)
	}
	if len(body.Rankings) != 0 {
		t.Errorf("len(Rankings) = %d, want 0", len(body.Rankings))
	}
	if body.OrganizationID != actor.orgID {
		t.Errorf("organization_id = %q, want %q", body.OrganizationID, actor.orgID)
	}
}

func TestRankings_Teams_Empty(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, teamRankingsURL(actor.orgSlug), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var body teamRankingsResponse
	decodeBody(t, resp, &body)
	if body.Total != 0 {
		t.Errorf("total = %d, want 0", body.Total)
	}
	if len(body.Rankings) != 0 {
		t.Errorf("len(Rankings) = %d, want 0", len(body.Rankings))
	}
}

// ── list with stats (direct DB insert) ───────────────────────────────────────

// TestRankings_Teams_WithStats verifies ranking order and field correctness after
// inserting stats directly into team_tournament_stats.
func TestRankings_Teams_WithStats(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	ctx := context.Background()
	orgUID := pgtype.UUID{}
	if err := orgUID.Scan(actor.orgID); err != nil {
		t.Fatalf("parse orgID: %v", err)
	}

	// Three teams: gold > silver > bronze based on tournaments_won.
	gold, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	silver, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	bronze, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)

	tourn1 := fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusCompleted)
	tourn2 := fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusCompleted)

	// gold: won 2 tournaments (position=1 in both)
	insertTeamStats(ctx, t, ts, orgUID, tourn1.ID, gold.ID, 1, 3, 3, 0, 0, 9)
	insertTeamStats(ctx, t, ts, orgUID, tourn2.ID, gold.ID, 1, 3, 3, 0, 0, 9)
	// silver: won 1 tournament (position=1 in tourn1), podium in tourn2
	insertTeamStats(ctx, t, ts, orgUID, tourn1.ID, silver.ID, 2, 3, 1, 1, 1, 4)
	insertTeamStats(ctx, t, ts, orgUID, tourn2.ID, silver.ID, 1, 3, 3, 0, 0, 9)
	// bronze: podium only
	insertTeamStats(ctx, t, ts, orgUID, tourn1.ID, bronze.ID, 3, 3, 0, 1, 2, 1)

	resp := ts.get(t, teamRankingsURL(actor.orgSlug), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var body teamRankingsResponse
	decodeBody(t, resp, &body)

	if body.Total != 3 {
		t.Fatalf("total = %d, want 3", body.Total)
	}
	if len(body.Rankings) != 3 {
		t.Fatalf("len(Rankings) = %d, want 3", len(body.Rankings))
	}

	// Rank 1 = gold (2 wins), Rank 2 = silver (1 win), Rank 3 = bronze (0 wins).
	goldID := pgutil.UUIDToString(gold.ID)
	silverID := pgutil.UUIDToString(silver.ID)
	bronzeID := pgutil.UUIDToString(bronze.ID)

	if body.Rankings[0].TeamID != goldID {
		t.Errorf("rank1 teamID = %q, want gold %q", body.Rankings[0].TeamID, goldID)
	}
	if body.Rankings[0].Rank != 1 {
		t.Errorf("rank1.Rank = %d, want 1", body.Rankings[0].Rank)
	}
	if body.Rankings[0].TournamentsWon != 2 {
		t.Errorf("rank1.TournamentsWon = %d, want 2", body.Rankings[0].TournamentsWon)
	}
	if body.Rankings[1].TeamID != silverID {
		t.Errorf("rank2 teamID = %q, want silver %q", body.Rankings[1].TeamID, silverID)
	}
	if body.Rankings[2].TeamID != bronzeID {
		t.Errorf("rank3 teamID = %q, want bronze %q", body.Rankings[2].TeamID, bronzeID)
	}
}

// TestRankings_Players_WithStats verifies player ranking list populated via
// direct stats insert.
func TestRankings_Players_WithStats(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	ctx := context.Background()
	orgUID := pgtype.UUID{}
	if err := orgUID.Scan(actor.orgID); err != nil {
		t.Fatalf("parse orgID: %v", err)
	}

	p1 := fixtures.CreatePlayer(ctx, t, ts.pool, orgUID)
	p2 := fixtures.CreatePlayer(ctx, t, ts.pool, orgUID)

	tourn := fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusCompleted)

	insertPlayerStats(ctx, t, ts, orgUID, tourn.ID, p1.ID, 1, 3, 3, 0, 0, 9)
	insertPlayerStats(ctx, t, ts, orgUID, tourn.ID, p2.ID, 2, 3, 1, 1, 1, 4)

	resp := ts.get(t, playerRankingsURL(actor.orgSlug), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var body playerRankingsResponse
	decodeBody(t, resp, &body)

	if body.Total != 2 {
		t.Fatalf("total = %d, want 2", body.Total)
	}
	if body.Rankings[0].PlayerID != pgutil.UUIDToString(p1.ID) {
		t.Errorf("rank1 playerID = %q, want p1 %q", body.Rankings[0].PlayerID, pgutil.UUIDToString(p1.ID))
	}
	if body.Rankings[0].Rank != 1 {
		t.Errorf("rank1.Rank = %d, want 1", body.Rankings[0].Rank)
	}
	if body.Rankings[0].TournamentsWon != 1 {
		t.Errorf("rank1.TournamentsWon = %d, want 1", body.Rankings[0].TournamentsWon)
	}
	if body.Rankings[0].WinRate == 0 {
		t.Errorf("rank1.WinRate = 0, want > 0 (3/3 = 1.0)")
	}
}

// ── pagination ────────────────────────────────────────────────────────────────

func TestRankings_Teams_Pagination(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	ctx := context.Background()
	orgUID := pgtype.UUID{}
	if err := orgUID.Scan(actor.orgID); err != nil {
		t.Fatalf("parse orgID: %v", err)
	}

	// Insert 3 teams but request limit=2.
	for i := range 3 {
		team, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
		tourn := fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusCompleted)
		insertTeamStats(ctx, t, ts, orgUID, tourn.ID, team.ID, i+1, 3, 3-i, 0, i, 9-(i*3))
	}

	resp := ts.get(t, teamRankingsURL(actor.orgSlug)+"?limit=2&offset=0", bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var body teamRankingsResponse
	decodeBody(t, resp, &body)

	if body.Total != 3 {
		t.Errorf("total = %d, want 3", body.Total)
	}
	if len(body.Rankings) != 2 {
		t.Errorf("len(Rankings) = %d, want 2 (limit applied)", len(body.Rankings))
	}
	if body.Limit != 2 {
		t.Errorf("limit = %d, want 2", body.Limit)
	}
}

// ── snapshot-on-completion E2E ────────────────────────────────────────────────

// TestRankings_Snapshot_OnTournamentCompletion verifies that transitioning a
// tournament to completed via the HTTP API triggers the stats snapshot and
// the rankings list reflects those stats.
func TestRankings_Snapshot_OnTournamentCompletion(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	ctx := context.Background()
	orgUID := pgtype.UUID{}
	if err := orgUID.Scan(actor.orgID); err != nil {
		t.Fatalf("parse orgID: %v", err)
	}

	// Set up: ongoing tournament with two registered teams and one completed match.
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	tournID := pgutil.UUIDToString(setup.Tournament.ID)

	// Home team wins 2-0.
	fixtures.CreateCompletedMatch(ctx, t, ts.pool,
		orgUID, setup.Tournament.ID,
		setup.HomeTeam.ID, setup.AwayTeam.ID,
		2, 0, setup.HomeTeam.ID,
	)

	// Complete the tournament via HTTP — this triggers snapshotTournamentStats.
	patchResp := ts.patch(t, tournamentURL(actor.orgSlug, tournID),
		map[string]any{"status": "completed"},
		bearerHeader(actor.token),
	)
	defer patchResp.Body.Close()
	assertStatus(t, patchResp, http.StatusOK)

	// Rankings should now reflect the completed tournament stats.
	rankResp := ts.get(t, teamRankingsURL(actor.orgSlug), bearerHeader(actor.token))
	defer rankResp.Body.Close()
	assertStatus(t, rankResp, http.StatusOK)

	var body teamRankingsResponse
	decodeBody(t, rankResp, &body)

	if body.Total != 2 {
		t.Fatalf("total = %d, want 2 (both registered teams snapshotted)", body.Total)
	}

	// Rank 1 should be the home team (winner = 1 win, position 1).
	homeID := pgutil.UUIDToString(setup.HomeTeam.ID)
	if body.Rankings[0].TeamID != homeID {
		t.Errorf("rank1 teamID = %q, want home team %q", body.Rankings[0].TeamID, homeID)
	}
	if body.Rankings[0].TournamentsWon != 1 {
		t.Errorf("rank1.TournamentsWon = %d, want 1", body.Rankings[0].TournamentsWon)
	}
}

// ── fixtures helpers ──────────────────────────────────────────────────────────

func insertTeamStats(
	ctx context.Context, t testing.TB, ts *testServer,
	orgID, tournamentID, teamID pgtype.UUID,
	position, played, wins, draws, losses, points int,
) {
	t.Helper()
	_, err := ts.pool.Exec(ctx, `
		INSERT INTO team_tournament_stats
		    (team_id, tournament_id, organization_id, position, matches_played,
		     matches_won, matches_drawn, matches_lost, points, score_for, score_against)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 0, 0)
		ON CONFLICT (team_id, tournament_id) DO NOTHING`,
		teamID, tournamentID, orgID, position, played, wins, draws, losses, points,
	)
	if err != nil {
		t.Fatalf("insertTeamStats: %v", err)
	}
}

func insertPlayerStats(
	ctx context.Context, t testing.TB, ts *testServer,
	orgID, tournamentID, playerID pgtype.UUID,
	position, played, wins, draws, losses, points int,
) {
	t.Helper()
	_, err := ts.pool.Exec(ctx, `
		INSERT INTO player_tournament_stats
		    (player_id, tournament_id, organization_id, position, matches_played,
		     matches_won, matches_drawn, matches_lost, points, score_for, score_against)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 0, 0)
		ON CONFLICT (player_id, tournament_id) DO NOTHING`,
		playerID, tournamentID, orgID, position, played, wins, draws, losses, points,
	)
	if err != nil {
		t.Fatalf("insertPlayerStats: %v", err)
	}
}
