package rankings_integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/email"
	"github.com/4yushraman-jpg/playarena/internal/notifications"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	mw "github.com/4yushraman-jpg/playarena/internal/platform/middleware"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/rankings"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
	"github.com/4yushraman-jpg/playarena/internal/tournaments"
)

const testJWTSecret = "playarena-integration-test-jwt-secret-key!!"

type testServer struct {
	url  string
	pool *pgxpool.Pool
	cfg  *config.Config
}

func testConfig() *config.Config {
	return &config.Config{
		AppEnv:                 "development",
		AppBaseURL:             "http://localhost:8080",
		DatabaseURL:            "postgres://integration-test:placeholder/playarena_test",
		JWTSecret:              testJWTSecret,
		CORSAllowedOrigins:     []string{"https://allowed.example.com"},
		RateLimitEnabled:       true,
		RateLimitAuthRPS:       100,
		RateLimitAuthBurst:     200,
		CleanupIntervalMinutes: 60,
		EmailFromAddress:       "noreply@test.example.com",
		EmailFromName:          "PlayArena Test",
	}
}

func buildTestServer(t testing.TB, pool *pgxpool.Pool) *testServer {
	t.Helper()

	cfg := testConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	noopMailer := &email.NoOpProvider{}
	sender := email.NewSenderWithProvider(noopMailer, email.SenderConfig{
		FromAddress: cfg.EmailFromAddress,
		FromName:    cfg.EmailFromName,
		AppBaseURL:  cfg.AppBaseURL,
	}, logger)

	limiter := mw.NewIPRateLimiter(rate.Limit(100), 200)
	queries := db.New(pool)
	authz := auth.NewAuthorizationService(queries)

	notifRepo := notifications.NewRepository(queries, pool)
	notifSvc := notifications.NewService(notifRepo, nil, logger)

	// Rankings repo is wired into tournaments so snapshot-on-completion works.
	rankingsRepo := rankings.NewRepository(queries, pool)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

	authHandler := auth.RegisterRoutes(r, pool, cfg, logger, limiter, sender, nil)
	r.Group(func(r chi.Router) {
		r.Use(mw.BodySizeLimit(64 * 1024))
		tournaments.RegisterRoutes(r, pool, cfg, logger, authz, notifSvc, rankingsRepo)
		rankings.RegisterRoutes(r, pool, cfg, logger)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		srv.Close()
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = authHandler.DrainEmail(drainCtx)
		limiter.Stop()
	})

	return &testServer{url: srv.URL, pool: pool, cfg: cfg}
}

// ── response structs ──────────────────────────────────────────────────────────

type playerRankingEntry struct {
	Rank              int     `json:"rank"`
	PlayerID          string  `json:"player_id"`
	DisplayName       string  `json:"display_name"`
	TournamentsPlayed int     `json:"tournaments_played"`
	TournamentsWon    int     `json:"tournaments_won"`
	PodiumFinishes    int     `json:"podium_finishes"`
	TotalMatches      int     `json:"total_matches"`
	TotalWins         int     `json:"total_wins"`
	TotalPoints       int     `json:"total_points"`
	WinRate           float64 `json:"win_rate"`
}

type playerRankingsResponse struct {
	OrganizationID string               `json:"organization_id"`
	Rankings       []playerRankingEntry `json:"rankings"`
	Total          int64                `json:"total"`
	Limit          int                  `json:"limit"`
	Offset         int                  `json:"offset"`
}

type teamRankingEntry struct {
	Rank              int     `json:"rank"`
	TeamID            string  `json:"team_id"`
	TeamName          string  `json:"team_name"`
	TournamentsPlayed int     `json:"tournaments_played"`
	TournamentsWon    int     `json:"tournaments_won"`
	PodiumFinishes    int     `json:"podium_finishes"`
	TotalMatches      int     `json:"total_matches"`
	TotalWins         int     `json:"total_wins"`
	TotalPoints       int     `json:"total_points"`
	WinRate           float64 `json:"win_rate"`
}

type teamRankingsResponse struct {
	OrganizationID string             `json:"organization_id"`
	Rankings       []teamRankingEntry `json:"rankings"`
	Total          int64              `json:"total"`
	Limit          int                `json:"limit"`
	Offset         int                `json:"offset"`
}

type errResp struct {
	Error string `json:"error"`
}

// ── URL builders ──────────────────────────────────────────────────────────────

func playerRankingsURL(orgSlug string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/rankings/players", orgSlug)
}

func teamRankingsURL(orgSlug string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/rankings/teams", orgSlug)
}

func tournamentURL(orgSlug, tournamentID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/tournaments/%s", orgSlug, tournamentID)
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func (ts *testServer) get(t testing.TB, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.url+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func (ts *testServer) patch(t testing.TB, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPatch, ts.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", path, err)
	}
	return resp
}

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

// ── assertions ────────────────────────────────────────────────────────────────

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

func bearerHeader(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

// ── auth helpers ──────────────────────────────────────────────────────────────

type orgContext struct {
	token   string
	orgID   string
	orgSlug string
}

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
	if r.AccessToken == "" {
		t.Fatal("loginAs: empty access_token")
	}
	return r.AccessToken
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
