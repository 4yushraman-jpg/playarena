package notifications_integration_test

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
	"github.com/4yushraman-jpg/playarena/internal/notifications"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	mw "github.com/4yushraman-jpg/playarena/internal/platform/middleware"
	tournament_registrations "github.com/4yushraman-jpg/playarena/internal/tournament_registrations"
	"github.com/4yushraman-jpg/playarena/internal/tournaments"
)

const testJWTSecret = "playarena-integration-test-jwt-secret-key!!"

type testServer struct {
	url      string
	pool     *pgxpool.Pool
	cfg      *config.Config
	notifSvc *notifications.Service
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
	notifSvc := notifications.NewService(notifRepo, logger)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

	authHandler := auth.RegisterRoutes(r, pool, cfg, logger, limiter, sender)
	r.Group(func(r chi.Router) {
		r.Use(mw.BodySizeLimit(64 * 1024))
		notifications.RegisterRoutes(r, pool, cfg, logger, authz)
		tournaments.RegisterRoutes(r, pool, cfg, logger, authz, notifSvc)
		tournament_registrations.RegisterRoutes(r, pool, cfg, logger, authz, notifSvc)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		srv.Close()
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = authHandler.DrainEmail(drainCtx)
		limiter.Stop()
	})

	return &testServer{url: srv.URL, pool: pool, cfg: cfg, notifSvc: notifSvc}
}
