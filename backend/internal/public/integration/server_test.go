package public_integration_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/public"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

type testServer struct {
	url  string
	pool *pgxpool.Pool
}

// buildTestServer mounts ONLY the public module — no auth anywhere — exactly as
// it is mounted in production (outside every authenticated group).
func buildTestServer(t testing.TB, pool *pgxpool.Pool) *testServer {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.Recoverer)
	public.RegisterRoutes(r, pool, logger, nil)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &testServer{url: srv.URL, pool: pool}
}

func (ts *testServer) get(t testing.TB, path string) *http.Response {
	t.Helper()
	// No Authorization header — the public path is strictly anonymous.
	resp, err := http.Get(ts.url + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func assertStatus(t testing.TB, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected HTTP %d, got %d; body: %s", want, resp.StatusCode, body)
	}
}

func decode(t testing.TB, resp *http.Response, dest any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func rawBody(t testing.TB, resp *http.Response) string {
	t.Helper()
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return string(b)
}

func setVisibility(t testing.TB, pool *pgxpool.Pool, tournamentID pgtype.UUID, v string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE tournaments SET visibility = $1 WHERE id = $2", v, tournamentID); err != nil {
		t.Fatalf("set visibility: %v", err)
	}
}

func setOrgStatus(t testing.TB, pool *pgxpool.Pool, orgID pgtype.UUID, status string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE organizations SET status = $1 WHERE id = $2", status, orgID); err != nil {
		t.Fatalf("set org status: %v", err)
	}
}

// publicURL builds the canonical public API path.
func publicURL(orgSlug, tournamentSlug string) string {
	return "/api/v1/public/orgs/" + orgSlug + "/tournaments/" + tournamentSlug
}

// seedOngoingTournament creates an org + an ongoing tournament + two registered
// teams, returning the org, tournament, and team handles for assertions.
type seed struct {
	org      db.Organization
	tourn    db.Tournament
	homeTeam db.Team
	awayTeam db.Team
}

func seedOngoingTournament(t testing.TB, pool *pgxpool.Pool) seed {
	t.Helper()
	ctx := context.Background()
	org := fixtures.CreateOrg(ctx, t, pool)
	tourn := fixtures.CreateTournament(ctx, t, pool, org.ID, db.TournamentStatusOngoing)
	home := fixtures.CreateTeam(ctx, t, pool, org.ID)
	away := fixtures.CreateTeam(ctx, t, pool, org.ID)
	fixtures.CreateApprovedRegistration(ctx, t, pool, org.ID, tourn.ID, home.ID)
	fixtures.CreateApprovedRegistration(ctx, t, pool, org.ID, tourn.ID, away.ID)
	return seed{org: org, tourn: tourn, homeTeam: home, awayTeam: away}
}
