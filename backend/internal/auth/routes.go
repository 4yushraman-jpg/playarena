package auth

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts all auth endpoints onto r under /api/v1/auth.
//
// Public routes (no auth required):
//
//	POST /api/v1/auth/register
//	GET  /api/v1/auth/verify-email
//	POST /api/v1/auth/login
//	POST /api/v1/auth/refresh
//	POST /api/v1/auth/logout
//
// Auth-only routes (RequireAuth):
//
//	GET  /api/v1/auth/me
//
// Auth + permission routes (RequireAuth + RequirePermission):
//
//	GET  /api/v1/auth/admin-only  — requires "role.assign" permission
//	                                (demonstrates the authorization layer)
func RegisterRoutes(r chi.Router, pool *pgxpool.Pool, cfg *config.Config, log *slog.Logger) {
	queries := db.New(pool)
	repo := NewRepository(queries, pool)
	svc := NewService(repo, cfg)
	authz := NewAuthorizationService(queries)
	h := NewHandler(svc, cfg, log)

	r.Route("/api/v1/auth", func(r chi.Router) {
		// ── public ──────────────────────────────────────────────────────────
		r.Post("/register", h.Register)
		r.Get("/verify-email", h.VerifyEmail)
		r.Post("/login", h.Login)
		r.Post("/refresh", h.Refresh)
		r.Post("/logout", h.Logout)

		// ── authenticated ────────────────────────────────────────────────────
		r.Group(func(r chi.Router) {
			r.Use(RequireAuth(cfg))
			r.Get("/me", h.Me)
		})

		// ── authenticated + permission check (RBAC demonstration) ────────────
		// GET /api/v1/auth/admin-only requires the "role.assign" permission.
		// Returns 401 when no valid token is present.
		// Returns 403 when the token is valid but the user lacks the permission.
		// Returns 200 when authentication and authorization both succeed.
		r.Group(func(r chi.Router) {
			r.Use(RequireAuth(cfg))
			r.Use(RequirePermission(authz, "role.assign"))
			r.Get("/admin-only", h.AdminOnly)
		})
	})
}
