package users_integration_test

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/users"
)

const (
	testJWTSecret = "playarena-integration-test-jwt-secret-key!!"
)

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
		CleanupIntervalMinutes: 60,
	}
}

// buildTestServer creates an httptest.Server with the users routes mounted.
// The middleware stack mirrors production: RequestID → RealIP → Recoverer.
// RequireAuth is applied inside users.RegisterRoutes.
func buildTestServer(t testing.TB, pool *pgxpool.Pool) *testServer {
	t.Helper()
	cfg := testConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

	users.RegisterRoutes(r, pool, cfg, logger)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &testServer{url: srv.URL, pool: pool, cfg: cfg}
}
