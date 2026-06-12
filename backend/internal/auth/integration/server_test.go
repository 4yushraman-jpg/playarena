package auth_integration_test

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

	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/email"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	mw "github.com/4yushraman-jpg/playarena/internal/platform/middleware"
)

const (
	// testJWTSecret is used to sign all access tokens issued by the test server.
	// It satisfies the minimum-32-character length check in config.validate.
	testJWTSecret = "playarena-integration-test-jwt-secret-key!!"

	// testAllowedOrigin is the single origin allowed by the test CORS config.
	testAllowedOrigin = "https://allowed.example.com"

	// testDisallowedOrigin is an origin that the test server will reject.
	testDisallowedOrigin = "https://evil.example.com"
)

// testServer wraps an httptest.Server with the pool and config used to build it.
type testServer struct {
	url     string
	pool    *pgxpool.Pool
	cfg     *config.Config
	limiter *mw.IPRateLimiter
	mailer  *email.NoOpProvider // inspect sent emails in tests; nil when server has no sender
	handler *auth.Handler       // for DrainEmail in cleanup
}

// testConfig returns a Config appropriate for integration tests.
// Development mode is on so that tokens appear in response bodies.
// DatabaseURL is a non-empty placeholder; the real connection comes from the pool.
func testConfig() *config.Config {
	return &config.Config{
		AppEnv:                 "development",
		AppBaseURL:             "http://localhost:3000",
		DatabaseURL:            "postgres://integration-test:placeholder/playarena_test",
		JWTSecret:              testJWTSecret,
		CORSAllowedOrigins:     []string{testAllowedOrigin},
		RateLimitEnabled:       true,
		RateLimitAuthRPS:       100,
		RateLimitAuthBurst:     200,
		CleanupIntervalMinutes: 60,
		EmailFromAddress:       "noreply@test.example.com",
		EmailFromName:          "PlayArena Test",
	}
}

// buildTestServer creates a test HTTP server with a permissive rate limiter
// (100 RPS, burst 200) and a NoOpProvider email sender. Intended for all
// non-rate-limit tests.
func buildTestServer(t testing.TB, pool *pgxpool.Pool) *testServer {
	t.Helper()
	return buildServerWithLimiter(t, pool, mw.NewIPRateLimiter(rate.Limit(100), 200))
}

// buildRateLimitedTestServer creates a test HTTP server with the given rate
// limiter parameters. Each call returns a fresh server with its own limiter
// instance. Intended exclusively for rate-limit-specific tests.
func buildRateLimitedTestServer(t testing.TB, pool *pgxpool.Pool, rps float64, burst int) *testServer {
	t.Helper()
	return buildServerWithLimiter(t, pool, mw.NewIPRateLimiter(rate.Limit(rps), burst))
}

// buildServerWithLimiter is the shared construction function. It mounts the
// middleware stack that mirrors the production bootstrap.NewRouter:
//
//	RequestID → RealIP → Recoverer → CORS → auth routes (with limiter)
//
// The RequestLogger middleware is omitted to keep test output clean.
func buildServerWithLimiter(t testing.TB, pool *pgxpool.Pool, limiter *mw.IPRateLimiter) *testServer {
	t.Helper()

	cfg := testConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	noopMailer := &email.NoOpProvider{}
	sender := email.NewSenderWithProvider(noopMailer, email.SenderConfig{
		FromAddress: cfg.EmailFromAddress,
		FromName:    cfg.EmailFromName,
		AppBaseURL:  cfg.AppBaseURL,
	}, logger)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(mw.CORS(cfg.CORSAllowedOrigins))

	authHandler := auth.RegisterRoutes(r, pool, cfg, logger, limiter, sender, nil)

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		srv.Close()
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = authHandler.DrainEmail(drainCtx)
		limiter.Stop()
	})

	return &testServer{
		url:     srv.URL,
		pool:    pool,
		cfg:     cfg,
		limiter: limiter,
		mailer:  noopMailer,
		handler: authHandler,
	}
}

// buildServerWithNilSender creates a test server with no email sender configured.
// Use this to verify that service logic (timing equalization, token creation)
// still executes when emailSender is nil — regression guard for P1-3.
func buildServerWithNilSender(t testing.TB, pool *pgxpool.Pool) *testServer {
	t.Helper()

	cfg := testConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	limiter := mw.NewIPRateLimiter(rate.Limit(100), 200)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(mw.CORS(cfg.CORSAllowedOrigins))

	authHandler := auth.RegisterRoutes(r, pool, cfg, logger, limiter, nil, nil)

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		srv.Close()
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = authHandler.DrainEmail(drainCtx)
		limiter.Stop()
	})

	return &testServer{
		url:     srv.URL,
		pool:    pool,
		cfg:     cfg,
		limiter: limiter,
		mailer:  nil, // no sender configured
		handler: authHandler,
	}
}
