package media_integration_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/email"
	"github.com/4yushraman-jpg/playarena/internal/media"
	"github.com/4yushraman-jpg/playarena/internal/media/storage"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	mw "github.com/4yushraman-jpg/playarena/internal/platform/middleware"
)

const testJWTSecret = "playarena-integration-test-jwt-secret-key!!"

type testServer struct {
	url     string
	pool    *pgxpool.Pool
	cfg     *config.Config
	uploads string // temporary upload directory
}

func testConfig(uploadsDir string) *config.Config {
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
		StorageBackend:         "local",
		StorageLocalPath:       uploadsDir,
		StorageLocalBaseURL:    "http://localhost:8080/media/files",
	}
}

func buildTestServer(t testing.TB, pool *pgxpool.Pool) *testServer {
	t.Helper()

	uploadsDir, err := os.MkdirTemp("", "playarena-media-test-*")
	if err != nil {
		t.Fatalf("buildTestServer: create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(uploadsDir) })

	cfg := testConfig(uploadsDir)
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

	backend, err := storage.New(cfg)
	if err != nil {
		t.Fatalf("buildTestServer: storage.New: %v", err)
	}

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

	authHandler := auth.RegisterRoutes(r, pool, cfg, logger, limiter, sender, nil)
	r.Group(func(r chi.Router) {
		r.Use(mw.BodySizeLimit(32 * 1024 * 1024)) // 32 MB for multipart
		media.RegisterRoutes(r, pool, cfg, logger, authz, backend)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		srv.Close()
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = authHandler.DrainEmail(drainCtx)
		limiter.Stop()
	})

	return &testServer{url: srv.URL, pool: pool, cfg: cfg, uploads: uploadsDir}
}
