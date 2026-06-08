package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/cleanup"
	"github.com/4yushraman-jpg/playarena/internal/notifworker"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/metrics"
	"github.com/4yushraman-jpg/playarena/internal/platform/middleware"
	"github.com/4yushraman-jpg/playarena/internal/realtime"
	"github.com/4yushraman-jpg/playarena/internal/webhookworker"
)

// App is the composition root of the PlayArena backend.
// It holds all top-level dependencies, assembles the HTTP handler, and owns
// the lifecycle of background services (rate-limiter cleanup, token cleanup,
// notification email delivery).
type App struct {
	Config *config.Config
	DB     *pgxpool.Pool
	Log    *slog.Logger

	reg                *metrics.Registry
	internalServer     *http.Server
	scraperDone        chan struct{}
	scheduler          *cleanup.Scheduler
	authLimiter        *middleware.IPRateLimiter    // /api/v1/auth/* — most restrictive
	writeLimiter       *middleware.IPRateLimiter    // domain write endpoints (POST/PATCH/DELETE)
	mediaLimiter       *middleware.IPRateLimiter    // media upload endpoint
	authHandler        *auth.Handler                // for DrainEmail on graceful shutdown
	notifEmailWorker   *notifworker.EmailWorker     // for Stop/Drain on graceful shutdown
	notifWebhookWorker *webhookworker.WebhookWorker // for Stop/Drain on graceful shutdown
	realtimeHub        *realtime.Hub                // for Shutdown on graceful shutdown
}

// Handler returns the fully-wired HTTP handler for the application.
//
// It also initialises and starts background services:
//   - Per-IP rate-limiter cleanup goroutines (when rate limiting is enabled).
//   - The token cleanup scheduler.
//   - The internal observability server (metrics, ready, live).
//   - The DB pool and outbox metrics scrapers.
//
// Handler must be called exactly once. Call Shutdown() to stop background
// services before the process exits.
func (a *App) Handler() http.Handler {
	// Metrics registry — constructed once, shared across all components.
	a.reg = metrics.New()

	// Rate limiters — constructed only when rate limiting is enabled in config.
	if a.Config.RateLimitEnabled {
		a.authLimiter = middleware.NewIPRateLimiter(
			rate.Limit(a.Config.RateLimitAuthRPS),
			a.Config.RateLimitAuthBurst,
		).WithMetrics(a.reg, "auth")
		a.writeLimiter = middleware.NewIPRateLimiter(
			rate.Limit(a.Config.RateLimitWriteRPS),
			a.Config.RateLimitWriteBurst,
		).WithMetrics(a.reg, "write")
		a.mediaLimiter = middleware.NewIPRateLimiter(
			rate.Limit(a.Config.RateLimitMediaRPS),
			a.Config.RateLimitMediaBurst,
		).WithMetrics(a.reg, "media")
		a.Log.Info("rate limiters started",
			slog.Float64("auth_rps", a.Config.RateLimitAuthRPS),
			slog.Float64("write_rps", a.Config.RateLimitWriteRPS),
			slog.Float64("media_rps", a.Config.RateLimitMediaRPS),
		)
	}

	// Cleanup scheduler — removes expired tokens and old audit logs.
	interval := time.Duration(a.Config.CleanupIntervalMinutes) * time.Minute
	a.scheduler = cleanup.New(db.New(a.DB), a.DB, interval, a.Config.AuditLogRetentionDays, a.Log)
	a.scheduler.Start()
	a.Log.Info("cleanup scheduler started", slog.String("interval", interval.String()))

	// DB pool scraper + outbox metrics scraper — share a single done channel.
	a.scraperDone = make(chan struct{})
	startDBPoolScraper(a.DB, a.reg, a.scraperDone)

	handler, authH, emailWorker, webhookWorker, hub, notifRepo, webhookRepo := NewRouter(a.DB, a.Log, a.Config, a.reg, a.authLimiter, a.writeLimiter, a.mediaLimiter)
	a.authHandler = authH
	a.realtimeHub = hub
	a.notifEmailWorker = emailWorker
	a.notifEmailWorker.Start()
	a.Log.Info("notification email worker started",
		slog.Int("interval_seconds", a.Config.NotifWorkerIntervalSeconds),
	)
	a.notifWebhookWorker = webhookWorker
	a.notifWebhookWorker.Start()
	a.Log.Info("webhook worker started",
		slog.Int("interval_seconds", a.Config.WebhookWorkerIntervalSeconds),
	)
	a.Log.Info("realtime hub started")

	// Outbox metrics scraper — started after repos are available.
	startOutboxMetricsScraper(notifRepo, webhookRepo, a.reg, a.Log, a.scraperDone)

	// Internal observability server — separate port, never exposed publicly.
	a.internalServer = newInternalServer(a.Config, a.reg, a.DB, a.Log)
	go func() {
		a.Log.Info("internal server listening", slog.String("addr", a.internalServer.Addr))
		if err := a.internalServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.Log.Error("internal server error", slog.Any("error", err))
		}
	}()
	a.Log.Info("internal observability server started",
		slog.Int("port", a.Config.AppInternalPort),
	)

	return handler
}

// InternalAddr returns the address the internal observability server is
// listening on. Used by callers (e.g. main) that need to shut it down.
func (a *App) InternalAddr() string {
	if a.internalServer == nil {
		return ""
	}
	return a.internalServer.Addr
}

// ShutdownInternal shuts down the internal observability server gracefully.
func (a *App) ShutdownInternal(ctx context.Context) {
	if a.internalServer == nil {
		return
	}
	if err := a.internalServer.Shutdown(ctx); err != nil {
		a.Log.Warn("internal server shutdown error", slog.Any("error", err))
	}
}

// Shutdown stops background services. It drains in-flight email goroutines
// before stopping the rate limiters. Safe to call multiple times.
func (a *App) Shutdown(ctx context.Context) {
	// Stop background scrapers.
	if a.scraperDone != nil {
		select {
		case <-a.scraperDone:
		default:
			close(a.scraperDone)
		}
	}

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
	if a.notifEmailWorker != nil {
		a.notifEmailWorker.Stop()
		if err := a.notifEmailWorker.Drain(ctx); err != nil {
			a.Log.Warn("shutdown: notification email worker drain failed",
				slog.String("error", err.Error()),
			)
		}
	}
	if a.notifWebhookWorker != nil {
		a.notifWebhookWorker.Stop()
		if err := a.notifWebhookWorker.Drain(ctx); err != nil {
			a.Log.Warn("shutdown: webhook worker drain failed",
				slog.String("error", err.Error()),
			)
		}
	}
	if a.realtimeHub != nil {
		a.realtimeHub.Shutdown()
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
