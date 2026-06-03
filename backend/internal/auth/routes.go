package auth

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/middleware"
)

// RegisterRoutes mounts all auth endpoints onto r under /api/v1/auth.
//
// Rate limiting is applied to the entire /api/v1/auth route group when limiter
// is non-nil (controlled by RateLimitEnabled in config).
//
// Public routes (no auth required):
//
//	POST /api/v1/auth/register
//	GET  /api/v1/auth/verify-email
//	POST /api/v1/auth/login
//	POST /api/v1/auth/refresh
//	POST /api/v1/auth/logout
//	POST /api/v1/auth/forgot-password
//	POST /api/v1/auth/reset-password
//
// Auth-only routes (RequireAuth):
//
//	GET  /api/v1/auth/me
//
// Auth + permission routes (RequireAuth + RequirePermission):
//
//	GET  /api/v1/auth/admin-only  — requires "role.assign" permission
func RegisterRoutes(
	r chi.Router,
	pool *pgxpool.Pool,
	cfg *config.Config,
	log *slog.Logger,
	limiter *middleware.IPRateLimiter,
) {
	queries := db.New(pool)
	repo := NewRepository(queries, pool)
	svc := NewService(repo, cfg)
	authz := NewAuthorizationService(queries)
	h := NewHandler(svc, cfg, log)

	r.Route("/api/v1/auth", func(r chi.Router) {
		// Apply per-IP rate limiting to all auth routes when enabled.
		// chimw.RealIP (mounted in NewRouter) has already normalised
		// r.RemoteAddr to the true client IP before this middleware runs.
		if limiter != nil {
			r.Use(limiter.Middleware())
		}

		// ── public ──────────────────────────────────────────────────────────
		r.Post("/register", h.Register)
		r.Get("/verify-email", h.VerifyEmail)
		r.Post("/login", h.Login)
		r.Post("/refresh", h.Refresh)
		r.Post("/logout", h.Logout)
		r.Post("/forgot-password", h.ForgotPassword)
		r.Post("/reset-password", h.ResetPassword)

		// ── authenticated ────────────────────────────────────────────────────
		r.Group(func(r chi.Router) {
			r.Use(RequireAuth(cfg))
			r.Get("/me", h.Me)
		})

		// ── authenticated + permission check ─────────────────────────────────
		r.Group(func(r chi.Router) {
			r.Use(RequireAuth(cfg))
			r.Use(RequirePermission(authz, "role.assign"))
			r.Get("/admin-only", h.AdminOnly)
		})
	})
}
