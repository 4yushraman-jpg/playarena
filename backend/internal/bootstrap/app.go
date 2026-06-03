package bootstrap

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/cleanup"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/middleware"
)

// App is the composition root of the PlayArena backend.
// It holds all top-level dependencies, assembles the HTTP handler, and owns
// the lifecycle of background services (rate-limiter cleanup, token cleanup).
type App struct {
	Config *config.Config
	DB     *pgxpool.Pool
	Log    *slog.Logger

	scheduler   *cleanup.Scheduler
	rateLimiter *middleware.IPRateLimiter
}

// Handler returns the fully-wired HTTP handler for the application.
//
// It also initialises and starts background services:
//   - The per-IP rate-limiter cleanup goroutine (when rate limiting is enabled).
//   - The token cleanup scheduler.
//
// Handler must be called exactly once. Call Shutdown() to stop background
// services before the process exits.
func (a *App) Handler() http.Handler {
	// Rate limiter — constructed only when enabled in config.
	if a.Config.RateLimitEnabled {
		a.rateLimiter = middleware.NewIPRateLimiter(
			rate.Limit(a.Config.RateLimitAuthRPS),
			a.Config.RateLimitAuthBurst,
		)
		a.Log.Info("rate limiter started",
			slog.Float64("rps", a.Config.RateLimitAuthRPS),
			slog.Int("burst", a.Config.RateLimitAuthBurst),
		)
	}

	// Cleanup scheduler — removes expired tokens on a configurable interval.
	interval := time.Duration(a.Config.CleanupIntervalMinutes) * time.Minute
	a.scheduler = cleanup.New(db.New(a.DB), interval, a.Log)
	a.scheduler.Start()
	a.Log.Info("cleanup scheduler started", slog.String("interval", interval.String()))

	return NewRouter(a.DB, a.Log, a.Config, a.rateLimiter)
}

// Shutdown stops background services. It should be called after the HTTP
// server has stopped accepting new connections but before the process exits.
// Safe to call multiple times.
func (a *App) Shutdown(_ context.Context) {
	if a.scheduler != nil {
		a.scheduler.Stop()
	}
	if a.rateLimiter != nil {
		a.rateLimiter.Stop()
	}
}
