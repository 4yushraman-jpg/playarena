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
func NewRouter(
	db *pgxpool.Pool,
	log *slog.Logger,
	cfg *config.Config,
	limiter *middleware.IPRateLimiter,
) http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)                         // Attaches X-Request-ID to every request/response
	r.Use(chimw.RealIP)                            // Populates RemoteAddr from X-Forwarded-For / X-Real-IP
	r.Use(chimw.Recoverer)                         // Catches panics, logs stack trace, returns 500
	r.Use(middleware.RequestLogger(log))           // Structured per-request logging via slog
	r.Use(middleware.CORS(cfg.CORSAllowedOrigins)) // Cross-Origin Resource Sharing headers

	registerModules(r, db, log, cfg, limiter)

	return r
}
