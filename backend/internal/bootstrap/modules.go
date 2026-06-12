package bootstrap

import (
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/email"
	"github.com/4yushraman-jpg/playarena/internal/health"
	"github.com/4yushraman-jpg/playarena/internal/match_events"
	"github.com/4yushraman-jpg/playarena/internal/matches"
	"github.com/4yushraman-jpg/playarena/internal/media"
	mediastorage "github.com/4yushraman-jpg/playarena/internal/media/storage"
	"github.com/4yushraman-jpg/playarena/internal/members"
	"github.com/4yushraman-jpg/playarena/internal/notifications"
	"github.com/4yushraman-jpg/playarena/internal/notifworker"
	"github.com/4yushraman-jpg/playarena/internal/organizations"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/metrics"
	"github.com/4yushraman-jpg/playarena/internal/platform/middleware"
	"github.com/4yushraman-jpg/playarena/internal/players"
	"github.com/4yushraman-jpg/playarena/internal/rankings"
	"github.com/4yushraman-jpg/playarena/internal/realtime"
	"github.com/4yushraman-jpg/playarena/internal/teams"
	"github.com/4yushraman-jpg/playarena/internal/tournament_registrations"
	"github.com/4yushraman-jpg/playarena/internal/tournaments"
	"github.com/4yushraman-jpg/playarena/internal/users"
	"github.com/4yushraman-jpg/playarena/internal/webhooks"
	"github.com/4yushraman-jpg/playarena/internal/webhookworker"
)

// registerModules wires all domain modules into the router and returns the auth
// Handler, EmailWorker, WebhookWorker, realtime Hub, and the notification and
// webhook repositories (used by the background metrics scrapers).
//
// authLimiter   — per-IP limiter for /api/v1/auth/* (applied inside auth.RegisterRoutes)
// writeLimiter  — per-IP limiter for domain write endpoints (POST/PATCH/DELETE)
// mediaLimiter  — per-IP limiter for media upload (most expensive per-request operation)
// All limiter parameters are nil-safe.
func registerModules(
	r chi.Router,
	pool *pgxpool.Pool,
	log *slog.Logger,
	cfg *config.Config,
	reg *metrics.Registry,
	authLimiter *middleware.IPRateLimiter,
	writeLimiter *middleware.IPRateLimiter,
	mediaLimiter *middleware.IPRateLimiter,
) (*auth.Handler, *notifworker.EmailWorker, *webhookworker.WebhookWorker, *realtime.Hub, *notifications.Repository, *webhookworker.Repository) {
	queries := db.New(pool)
	authz := auth.NewAuthorizationService(queries)

	hub := realtime.NewHub().WithMetrics(reg)

	notifRepo := notifications.NewRepository(queries, pool)
	notifSvc := notifications.NewService(notifRepo, hub, log).WithMetrics(reg, cfg.DrainTimeoutSeconds)

	// Email sender — constructed once and shared with the auth module.
	// Failure here is fatal: a misconfigured email provider is a deployment
	// error, not a runtime recoverable condition (mirrors media storage panic).
	emailSender, err := email.NewSender(cfg, log)
	if err != nil {
		log.Error("bootstrap: failed to initialise email sender",
			slog.String("provider", cfg.EmailProvider),
			slog.Any("error", err),
		)
		panic("email sender initialisation failed: " + err.Error())
	}
	log.Info("email sender initialised", slog.String("provider", cfg.EmailProvider))

	// Email worker — delivers pending email channel notification rows async.
	workerInterval := time.Duration(cfg.NotifWorkerIntervalSeconds) * time.Second
	emailWorker := notifworker.NewEmailWorker(pool, emailSender, cfg.AppBaseURL, workerInterval, log, reg)

	// Webhook worker — delivers pending webhook_deliveries rows async.
	webhookInterval := time.Duration(cfg.WebhookWorkerIntervalSeconds) * time.Second
	webhookWorker, err := webhookworker.NewWebhookWorker(pool, cfg.WebhookSecretKey, nil, webhookInterval, log, reg)
	if err != nil {
		log.Error("bootstrap: failed to initialise webhook worker",
			slog.Any("error", err),
		)
		panic("webhook worker initialisation failed: " + err.Error())
	}
	webhookRepo := webhookworker.NewRepository(pool)

	rankingsRepo := rankings.NewRepository(queries, pool)

	health.RegisterRoutes(r, pool)
	authHandler := auth.RegisterRoutes(r, pool, cfg, log, authLimiter, emailSender, reg)

	// Domain write endpoints — writeLimiter applied to POST/PUT/PATCH/DELETE.
	// GET requests pass through without consuming tokens.
	r.Group(func(r chi.Router) {
		// Cap request bodies at 64 KB to prevent OOM from adversarial oversized
		// JSON payloads. Applied before the rate limiter so malformed large bodies
		// are rejected cheaply. Media upload routes are excluded — they manage
		// their own size limit inside the handler.
		r.Use(middleware.BodySizeLimit(64 * 1024))
		if writeLimiter != nil {
			r.Use(writeLimiter.WriteMiddleware())
		}
		users.RegisterRoutes(r, pool, cfg, log)
		organizations.RegisterRoutes(r, pool, cfg, log, authz)
		members.RegisterRoutes(r, pool, cfg, log, authz)
		players.RegisterRoutes(r, pool, cfg, log, authz)
		// GP-1: global PlayerProfile self-service (flag-gated inside).
		players.RegisterMeRoutes(r, pool, cfg, log)
		teams.RegisterRoutes(r, pool, cfg, log, authz)
		tournaments.RegisterRoutes(r, pool, cfg, log, authz, notifSvc, rankingsRepo)
		tournament_registrations.RegisterRoutes(r, pool, cfg, log, authz, notifSvc)
		matches.RegisterRoutes(r, pool, cfg, log, authz, notifSvc)
		match_events.RegisterRoutes(r, pool, cfg, log, authz)
		notifications.RegisterRoutes(r, pool, cfg, log, authz, hub)
		webhooks.RegisterRoutes(r, pool, cfg, log, authz)
		rankings.RegisterRoutes(r, pool, cfg, log)
	})

	mediaBackend, err := mediastorage.New(cfg)
	if err != nil {
		log.Error("bootstrap: failed to initialise media storage backend",
			slog.String("backend", cfg.StorageBackend),
			slog.Any("error", err),
		)
		panic("media storage backend initialisation failed: " + err.Error())
	}

	// Media upload — stricter mediaLimiter applied to POST/PUT/PATCH/DELETE.
	// Uploads trigger S3 writes and image processing; they are the most
	// expensive per-request operation and warrant a tighter per-IP budget.
	r.Group(func(r chi.Router) {
		if mediaLimiter != nil {
			r.Use(mediaLimiter.WriteMiddleware())
		}
		media.RegisterRoutes(r, pool, cfg, log, authz, mediaBackend)
	})

	return authHandler, emailWorker, webhookWorker, hub, notifRepo, webhookRepo
}
