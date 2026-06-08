package bootstrap

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/notifications"
	"github.com/4yushraman-jpg/playarena/internal/notifworker"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/metrics"
	"github.com/4yushraman-jpg/playarena/internal/platform/middleware"
	"github.com/4yushraman-jpg/playarena/internal/realtime"
	"github.com/4yushraman-jpg/playarena/internal/webhookworker"
)

// NewRouter builds and returns the fully-configured application HTTP router,
// the auth Handler (needed for DrainEmail on graceful shutdown), the
// EmailWorker, WebhookWorker, and realtime Hub (all needed for graceful
// shutdown), and the notification and webhook repositories (used by the
// background metrics scrapers).
//
// authLimiter — per-IP rate limiter for /api/v1/auth/* (most restrictive)
// writeLimiter — per-IP rate limiter for domain write endpoints (POST/PATCH/DELETE)
// mediaLimiter — per-IP rate limiter for media upload endpoint (most expensive)
// All limiter parameters are nil-safe: nil means no rate limiting for that group.
func NewRouter(
	db *pgxpool.Pool,
	log *slog.Logger,
	cfg *config.Config,
	reg *metrics.Registry,
	authLimiter *middleware.IPRateLimiter,
	writeLimiter *middleware.IPRateLimiter,
	mediaLimiter *middleware.IPRateLimiter,
) (http.Handler, *auth.Handler, *notifworker.EmailWorker, *webhookworker.WebhookWorker, *realtime.Hub, *notifications.Repository, *webhookworker.Repository) {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)                                 // Attaches X-Request-ID to every request/response
	r.Use(middleware.TrustedRealIP(cfg.TrustedProxyCIDRs)) // Rewrites RemoteAddr from forwarding headers only for trusted proxies
	r.Use(chimw.Recoverer)                                 // Catches panics, logs stack trace, returns 500
	r.Use(middleware.RequestLogger(log))                   // Structured per-request logging via slog
	r.Use(middleware.CORS(cfg.CORSAllowedOrigins))         // Cross-Origin Resource Sharing headers
	r.Use(middleware.Metrics(reg))                         // Prometheus HTTP metrics (counter + histogram + in-flight)

	authHandler, emailWorker, webhookWorker, hub, notifRepo, webhookRepo := registerModules(r, db, log, cfg, reg, authLimiter, writeLimiter, mediaLimiter)

	return r, authHandler, emailWorker, webhookWorker, hub, notifRepo, webhookRepo
}
