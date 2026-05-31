package auth

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts the auth endpoints onto r under /api/v1/auth.
//
// Public routes (no auth middleware):
//
//	POST /api/v1/auth/login
//	POST /api/v1/auth/refresh
//	POST /api/v1/auth/logout
//
// Protected routes (RequireAuth middleware):
//
//	GET  /api/v1/auth/me
func RegisterRoutes(r chi.Router, pool *pgxpool.Pool, cfg *config.Config, log *slog.Logger) {
	queries := db.New(pool)
	repo := NewRepository(queries, pool)
	svc := NewService(repo, cfg)
	h := NewHandler(svc, cfg, log)

	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/login", h.Login)
		r.Post("/refresh", h.Refresh)
		r.Post("/logout", h.Logout)

		r.Group(func(r chi.Router) {
			r.Use(RequireAuth(cfg))
			r.Get("/me", h.Me)
		})
	})
}
