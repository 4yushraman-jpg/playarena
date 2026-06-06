package users

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts all user endpoints onto r under /api/v1/users.
//
// All routes require a valid access token (RequireAuth). No permission
// middleware is applied at the route level — all authorization decisions
// are enforced inline by the service layer:
//
//	GET    /api/v1/users              → ListUsers       (platform admin only)
//	GET    /api/v1/users/{id}         → GetUser         (self or platform admin)
//	PATCH  /api/v1/users/{id}         → UpdateProfile   (self or platform admin)
//	POST   /api/v1/users/{id}/change-password → ChangePassword (self only)
//	POST   /api/v1/users/{id}/deactivate      → DeactivateUser (platform admin only)
func RegisterRoutes(
	r chi.Router,
	pool *pgxpool.Pool,
	cfg *config.Config,
	log *slog.Logger,
) {
	queries := db.New(pool)
	repo := NewRepository(queries, pool)
	svc := NewService(repo)
	h := NewHandler(svc, log)

	r.Route("/api/v1/users", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg))

		r.Get("/", h.ListUsers)

		r.Get("/{id}", h.GetUser)
		r.Patch("/{id}", h.UpdateProfile)

		r.Post("/{id}/change-password", h.ChangePassword)
		r.Post("/{id}/deactivate", h.DeactivateUser)
	})
}
