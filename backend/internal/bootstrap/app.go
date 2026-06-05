package bootstrap

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
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

	scheduler    *cleanup.Scheduler
	authLimiter  *middleware.IPRateLimiter // /api/v1/auth/* — most restrictive
	writeLimiter *middleware.IPRateLimiter // domain write endpoints (POST/PATCH/DELETE)
	mediaLimiter *middleware.IPRateLimiter // media upload endpoint
	authHandler  *auth.Handler             // for DrainEmail on graceful shutdown
}

// Handler returns the fully-wired HTTP handler for the application.
//
// It also initialises and starts background services:
//   - Per-IP rate-limiter cleanup goroutines (when rate limiting is enabled).
//   - The token cleanup scheduler.
//
// Handler must be called exactly once. Call Shutdown() to stop background
// services before the process exits.
func (a *App) Handler() http.Handler {
	// Rate limiters — constructed only when rate limiting is enabled in config.
	if a.Config.RateLimitEnabled {
		a.authLimiter = middleware.NewIPRateLimiter(
			rate.Limit(a.Config.RateLimitAuthRPS),
			a.Config.RateLimitAuthBurst,
		)
		a.writeLimiter = middleware.NewIPRateLimiter(
			rate.Limit(a.Config.RateLimitWriteRPS),
			a.Config.RateLimitWriteBurst,
		)
		a.mediaLimiter = middleware.NewIPRateLimiter(
			rate.Limit(a.Config.RateLimitMediaRPS),
			a.Config.RateLimitMediaBurst,
		)
		a.Log.Info("rate limiters started",
			slog.Float64("auth_rps", a.Config.RateLimitAuthRPS),
			slog.Float64("write_rps", a.Config.RateLimitWriteRPS),
			slog.Float64("media_rps", a.Config.RateLimitMediaRPS),
		)
	}

	// Cleanup scheduler — removes expired tokens on a configurable interval.
	interval := time.Duration(a.Config.CleanupIntervalMinutes) * time.Minute
	a.scheduler = cleanup.New(db.New(a.DB), interval, a.Log)
	a.scheduler.Start()
	a.Log.Info("cleanup scheduler started", slog.String("interval", interval.String()))

	handler, authH := NewRouter(a.DB, a.Log, a.Config, a.authLimiter, a.writeLimiter, a.mediaLimiter)
	a.authHandler = authH
	return handler
}

// Shutdown stops background services. It drains in-flight email goroutines
// before stopping the rate limiters. Safe to call multiple times.
func (a *App) Shutdown(ctx context.Context) {
	// Drain email goroutines first — they may still be running after the HTTP
	// server stops accepting connections. The provided ctx carries the overall
	// shutdown deadline; if it expires the drain is abandoned (goroutines will
	// eventually self-terminate via their own 30-second context timeout).
	if a.authHandler != nil {
		if err := a.authHandler.DrainEmail(ctx); err != nil {
			a.Log.Warn("shutdown: email drain timed out, some emails may not have been sent",
				slog.String("error", err.Error()),
			)
		}
	}
	if a.scheduler != nil {
		a.scheduler.Stop()
	}
	if a.authLimiter != nil {
		a.authLimiter.Stop()
	}
	if a.writeLimiter != nil {
		a.writeLimiter.Stop()
	}
	if a.mediaLimiter != nil {
		a.mediaLimiter.Stop()
	}
}
