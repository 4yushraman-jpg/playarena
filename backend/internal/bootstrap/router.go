package bootstrap

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/middleware"
)

// NewRouter builds and returns the fully-configured application HTTP router.
// Middleware is applied in registration order.
//
// authLimiter — per-IP rate limiter for /api/v1/auth/* (most restrictive)
// writeLimiter — per-IP rate limiter for domain write endpoints (POST/PATCH/DELETE)
// mediaLimiter — per-IP rate limiter for media upload endpoint (most expensive)
// All limiter parameters are nil-safe: nil means no rate limiting for that group.
func NewRouter(
	db *pgxpool.Pool,
	log *slog.Logger,
	cfg *config.Config,
	authLimiter *middleware.IPRateLimiter,
	writeLimiter *middleware.IPRateLimiter,
	mediaLimiter *middleware.IPRateLimiter,
) http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)                         // Attaches X-Request-ID to every request/response
	r.Use(chimw.RealIP)                            // Populates RemoteAddr from X-Forwarded-For / X-Real-IP
	r.Use(chimw.Recoverer)                         // Catches panics, logs stack trace, returns 500
	r.Use(middleware.RequestLogger(log))           // Structured per-request logging via slog
	r.Use(middleware.CORS(cfg.CORSAllowedOrigins)) // Cross-Origin Resource Sharing headers

	registerModules(r, db, log, cfg, authLimiter, writeLimiter, mediaLimiter)

	return r
}
