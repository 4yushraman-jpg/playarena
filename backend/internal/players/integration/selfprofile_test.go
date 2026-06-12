package players_integration_test

import (
	"context"
	"io"
	"log/slog"
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
	mw "github.com/4yushraman-jpg/playarena/internal/platform/middleware"
	"github.com/4yushraman-jpg/playarena/internal/players"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// buildPersonaServer mounts auth + org player routes + the GP-1 self-profile
// routes with PlayerPersonaEnabled=true.
func buildPersonaServer(t testing.TB, pool *pgxpool.Pool) *testServer {
	t.Helper()
	cfg := testConfig()
	cfg.PlayerPersonaEnabled = true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	sender := email.NewSenderWithProvider(&email.NoOpProvider{}, email.SenderConfig{
		FromAddress: cfg.EmailFromAddress, FromName: cfg.EmailFromName, AppBaseURL: cfg.AppBaseURL,
	}, logger)
	limiter := mw.NewIPRateLimiter(rate.Limit(100), 200)
	queries := db.New(pool)
	authz := auth.NewAuthorizationService(queries)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

	authHandler := auth.RegisterRoutes(r, pool, cfg, logger, limiter, sender, nil)
	r.Group(func(r chi.Router) {
		r.Use(mw.BodySizeLimit(64 * 1024))
		players.RegisterRoutes(r, pool, cfg, logger, authz)
		players.RegisterMeRoutes(r, pool, cfg, logger)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		srv.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = authHandler.DrainEmail(ctx)
		limiter.Stop()
	})
	return &testServer{url: srv.URL, pool: pool, cfg: cfg}
}

type meDoc struct {
	Scope           string `json:"scope"`
	PlayerProfileID string `json:"player_profile_id"`
}

func getMe(t testing.TB, ts *testServer, token string) meDoc {
	t.Helper()
	resp := ts.get(t, "/api/v1/auth/me", bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
	var d meDoc
	decodeBody(t, resp, &d)
	return d
}

// TestSelfProfile_CreateGetUpdate covers the happy path, 1:1 enforcement, and
// the player-scope login transition.
func TestSelfProfile_CreateGetUpdate(t *testing.T) {
	ts := buildPersonaServer(t, testPool)
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, ts.pool)

	// 0-org, no profile → onboarding token.
	onboardTok := loginAs(t, ts, user.Email, fixtures.KnownPasswordRaw, "")
	if me := getMe(t, ts, onboardTok); me.Scope != auth.ScopeOnboarding {
		t.Fatalf("expected onboarding scope, got %q", me.Scope)
	}

	// Create own profile.
	resp := ts.postWithHeaders(t, "/api/v1/me/player",
		map[string]any{"display_name": "Pawan", "visibility": "public"}, bearerHeader(onboardTok))
	assertStatus(t, resp, 201)
	var created playerResponse
	decodeBody(t, resp, &created)
	if created.OrganizationID != "" {
		t.Fatalf("global profile must have empty organization_id, got %q", created.OrganizationID)
	}

	// Second create → 409.
	resp = ts.postWithHeaders(t, "/api/v1/me/player",
		map[string]any{"display_name": "Dup"}, bearerHeader(onboardTok))
	assertStatus(t, resp, 409)

	// Re-login: 0 orgs + profile → player scope.
	playerTok := loginAs(t, ts, user.Email, fixtures.KnownPasswordRaw, "")
	me := getMe(t, ts, playerTok)
	if me.Scope != auth.ScopePlayer {
		t.Fatalf("expected player scope after profile creation, got %q", me.Scope)
	}
	if me.PlayerProfileID != created.ID {
		t.Fatalf("player_profile_id = %q, want %q", me.PlayerProfileID, created.ID)
	}

	// GET own.
	resp = ts.get(t, "/api/v1/me/player", bearerHeader(playerTok))
	assertStatus(t, resp, 200)
	resp.Body.Close()

	// PATCH own visibility.
	resp = ts.patch(t, "/api/v1/me/player", map[string]any{"visibility": "unlisted"}, bearerHeader(playerTok))
	assertStatus(t, resp, 200)
	resp.Body.Close()

	// PATCH immutable field → 422.
	resp = ts.patch(t, "/api/v1/me/player", map[string]any{"organization_id": "x"}, bearerHeader(playerTok))
	assertStatus(t, resp, 422)
	resp.Body.Close()
}

// TestSelfProfile_RefreshScopeEscalation asserts a player cannot escalate to
// platform or organizer scope on refresh (entitlement is re-verified).
func TestSelfProfile_RefreshScopeEscalation(t *testing.T) {
	ts := buildPersonaServer(t, testPool)
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, ts.pool)
	onboardTok := loginAs(t, ts, user.Email, fixtures.KnownPasswordRaw, "")
	resp := ts.postWithHeaders(t, "/api/v1/me/player",
		map[string]any{"display_name": "Climber"}, bearerHeader(onboardTok))
	assertStatus(t, resp, 201)
	resp.Body.Close()

	// Login again to obtain a player-scope session + its refresh token.
	loginResp := ts.post(t, "/api/v1/auth/login",
		map[string]any{"email": user.Email, "password": fixtures.KnownPasswordRaw})
	assertStatus(t, loginResp, 200)
	var lr struct {
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	decodeBody(t, loginResp, &lr)
	if lr.Scope != auth.ScopePlayer {
		t.Fatalf("expected player scope, got %q", lr.Scope)
	}

	// Escalation attempts → 403.
	for _, scope := range []string{"platform", "organizer"} {
		r := ts.post(t, "/api/v1/auth/refresh",
			map[string]any{"refresh_token": lr.RefreshToken, "scope": scope})
		assertStatus(t, r, 403)
		r.Body.Close()
	}

	// Legitimate player refresh (explicit scope) → 200.
	r := ts.post(t, "/api/v1/auth/refresh",
		map[string]any{"refresh_token": lr.RefreshToken, "scope": "player"})
	assertStatus(t, r, 200)
	r.Body.Close()
}

// TestSelfProfile_VisibilityAndIsolation covers visibility-aware reads and the
// player-token tenant boundary (403 on org-admin routes).
func TestSelfProfile_VisibilityAndIsolation(t *testing.T) {
	ts := buildPersonaServer(t, testPool)
	ctx := context.Background()

	owner := fixtures.CreateActiveUser(ctx, t, ts.pool)
	ownerOnboard := loginAs(t, ts, owner.Email, fixtures.KnownPasswordRaw, "")
	resp := ts.postWithHeaders(t, "/api/v1/me/player",
		map[string]any{"display_name": "Private Guy", "visibility": "private"}, bearerHeader(ownerOnboard))
	assertStatus(t, resp, 201)
	var prof playerResponse
	decodeBody(t, resp, &prof)
	ownerTok := loginAs(t, ts, owner.Email, fixtures.KnownPasswordRaw, "")

	viewer := fixtures.CreateActiveUser(ctx, t, ts.pool)
	viewerOnboard := loginAs(t, ts, viewer.Email, fixtures.KnownPasswordRaw, "")
	resp = ts.postWithHeaders(t, "/api/v1/me/player",
		map[string]any{"display_name": "Viewer"}, bearerHeader(viewerOnboard))
	assertStatus(t, resp, 201)
	resp.Body.Close()
	viewerTok := loginAs(t, ts, viewer.Email, fixtures.KnownPasswordRaw, "")

	// Private profile is invisible to a non-owner (404 hides existence).
	resp = ts.get(t, "/api/v1/players/"+prof.ID, bearerHeader(viewerTok))
	assertStatus(t, resp, 404)
	resp.Body.Close()

	// Owner can always read their own.
	resp = ts.get(t, "/api/v1/players/"+prof.ID, bearerHeader(ownerTok))
	assertStatus(t, resp, 200)
	resp.Body.Close()

	// Make public → visible to the viewer.
	resp = ts.patch(t, "/api/v1/me/player", map[string]any{"visibility": "public"}, bearerHeader(ownerTok))
	assertStatus(t, resp, 200)
	resp.Body.Close()
	resp = ts.get(t, "/api/v1/players/"+prof.ID, bearerHeader(viewerTok))
	assertStatus(t, resp, 200)
	resp.Body.Close()

	// Tenant boundary: a player token is rejected by an org-admin route (403),
	// regardless of slug existence (RequireOrgScope runs first).
	resp = ts.get(t, "/api/v1/organizations/any-slug/players", bearerHeader(viewerTok))
	assertStatus(t, resp, 403)
	resp.Body.Close()
}
