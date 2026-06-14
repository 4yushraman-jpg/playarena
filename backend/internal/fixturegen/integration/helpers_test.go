package fixturegen_integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── response shapes ─────────────────────────────────────────────────────────────

type previewSlot struct {
	Kind          string `json:"kind"`
	ParticipantID string `json:"participant_id"`
	Group         string `json:"group"`
	Rank          int    `json:"rank"`
}

type previewMatch struct {
	RoundNumber     int         `json:"round_number"`
	RoundName       string      `json:"round_name"`
	MatchNumber     int         `json:"match_number"`
	GroupLabel      string      `json:"group_label"`
	Home            previewSlot `json:"home"`
	Away            previewSlot `json:"away"`
	NextMatchNumber *int        `json:"next_match_number"`
	NextMatchSlot   *int        `json:"next_match_slot"`
}

type generateResponse struct {
	DryRun       bool           `json:"dry_run"`
	Generated    int            `json:"generated"`
	Format       string         `json:"format"`
	SeedStrategy string         `json:"seed_strategy"`
	RandomSeed   *int64         `json:"random_seed"`
	SeedOrder    []string       `json:"seed_order"`
	Matches      []previewMatch `json:"matches"`
	Warnings     []string       `json:"warnings"`
	GeneratedAt  *string        `json:"generated_at"`
}

type generationInfo struct {
	Generated    bool     `json:"generated"`
	GeneratedBy  string   `json:"generated_by"`
	Format       string   `json:"format"`
	SeedStrategy string   `json:"seed_strategy"`
	RandomSeed   *int64   `json:"random_seed"`
	GeneratedAt  string   `json:"generated_at"`
	MatchCount   int      `json:"match_count"`
	SeedOrder    []string `json:"seed_order"`
}

type resolveResponse struct {
	SlotsFilled int `json:"slots_filled"`
}

// ── auth + org ──────────────────────────────────────────────────────────────────

func loginAs(t testing.TB, ts *testServer, emailAddr, password, orgID string) string {
	t.Helper()
	body := map[string]any{"email": emailAddr, "password": password}
	if orgID != "" {
		body["organization_id"] = orgID
	}
	resp := ts.post(t, "/api/v1/auth/login", body)
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
	var r struct {
		AccessToken string `json:"access_token"`
	}
	decodeBody(t, resp, &r)
	return r.AccessToken
}

type orgContext struct {
	token   string
	orgID   string
	orgSlug string
}

func setupUserAndOrg(t testing.TB, ts *testServer, roleSlug string) orgContext {
	t.Helper()
	ctx := context.Background()
	user := fixtures.CreateActiveUser(ctx, t, ts.pool)
	org := fixtures.CreateOrgForUser(ctx, t, ts.pool, user.ID, roleSlug)
	orgIDStr := pgutil.UUIDToString(org.ID)
	token := loginAs(t, ts, user.Email, fixtures.KnownPasswordRaw, orgIDStr)
	return orgContext{token: token, orgID: orgIDStr, orgSlug: org.Slug}
}

func mustUUID(t testing.TB, s string) pgtype.UUID {
	t.Helper()
	uid, err := pgutil.ParseUUID(s)
	if err != nil {
		t.Fatalf("mustUUID %q: %v", s, err)
	}
	return uid
}

// seedClosedTournament creates a registration_closed tournament of the given
// format with nTeams approved team registrations, returning its id and team ids.
func seedClosedTournament(t testing.TB, ts *testServer, orgUID pgtype.UUID, format db.TournamentFormat, nTeams int) (string, []string) {
	t.Helper()
	ctx := context.Background()
	tourn := fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusRegistrationClosed)
	if _, err := ts.pool.Exec(ctx, "UPDATE tournaments SET format = $1 WHERE id = $2", string(format), tourn.ID); err != nil {
		t.Fatalf("set format: %v", err)
	}
	teamIDs := make([]string, 0, nTeams)
	for i := 0; i < nTeams; i++ {
		team := fixtures.CreateTeam(ctx, t, ts.pool, orgUID)
		fixtures.CreateApprovedRegistration(ctx, t, ts.pool, orgUID, tourn.ID, team.ID)
		teamIDs = append(teamIDs, pgutil.UUIDToString(team.ID))
	}
	return pgutil.UUIDToString(tourn.ID), teamIDs
}

func setTournamentStatus(t testing.TB, ts *testServer, tournamentID, status string) {
	t.Helper()
	if _, err := ts.pool.Exec(context.Background(),
		"UPDATE tournaments SET status = $1 WHERE id = $2", status, mustUUID(t, tournamentID)); err != nil {
		t.Fatalf("set tournament status: %v", err)
	}
}

// completeAllGroupMatches sets every group-stage match to completed with the
// home side winning, so group standings are well-defined for resolution.
func completeAllGroupMatches(t testing.TB, ts *testServer, tournamentID string) {
	t.Helper()
	_, err := ts.pool.Exec(context.Background(), `
		UPDATE matches
		SET    status = 'completed', started_at = NOW() - INTERVAL '1 hour', ended_at = NOW(),
		       home_score = 10, away_score = 0, winner_team_id = home_team_id
		WHERE  tournament_id = $1 AND group_label IS NOT NULL`,
		mustUUID(t, tournamentID))
	if err != nil {
		t.Fatalf("complete group matches: %v", err)
	}
}

func countMatches(t testing.TB, ts *testServer, tournamentID string) int {
	t.Helper()
	var n int
	if err := ts.pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM matches WHERE tournament_id = $1", mustUUID(t, tournamentID)).Scan(&n); err != nil {
		t.Fatalf("count matches: %v", err)
	}
	return n
}

// ── URLs ────────────────────────────────────────────────────────────────────────

func generateURL(orgSlug, tournamentID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/tournaments/%s/fixtures/generate", orgSlug, tournamentID)
}
func resolveURL(orgSlug, tournamentID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/tournaments/%s/fixtures/resolve-qualifiers", orgSlug, tournamentID)
}
func generationURL(orgSlug, tournamentID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/tournaments/%s/fixtures/generation", orgSlug, tournamentID)
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────────

func (ts *testServer) post(t testing.TB, path string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func (ts *testServer) postAuth(t testing.TB, path string, body any, token string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func (ts *testServer) getAuth(t testing.TB, path, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.url+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

// ── assertions ────────────────────────────────────────────────────────────────────

func assertStatus(t testing.TB, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected HTTP %d, got %d; body: %s", want, resp.StatusCode, body)
	}
}

func decodeBody(t testing.TB, resp *http.Response, dest any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		t.Fatalf("decodeBody: %v", err)
	}
}
