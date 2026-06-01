package players

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts all player endpoints under
// /api/v1/organizations/{slug}/players.
//
// Authorization matrix:
//
//	POST   /                  RequireAuth + RequirePermission("player.create")
//	GET    /                  RequireAuth
//	GET    /{id}              RequireAuth
//	PATCH  /{id}              RequireAuth + RequirePermission("player.update")
//	DELETE /{id}              RequireAuth + RequirePermission("player.delete")
func RegisterRoutes(
	r chi.Router,
	pool *pgxpool.Pool,
	cfg *config.Config,
	log *slog.Logger,
	authz *auth.AuthorizationService,
) {
	queries := db.New(pool)
	repo := NewRepository(queries, pool)
	svc := NewService(repo, log)
	h := NewHandler(svc, log)

	r.Route("/api/v1/organizations/{slug}/players", func(r chi.Router) {
		// All player routes require a valid access token.
		r.Use(auth.RequireAuth(cfg))

		// Create — requires player.create permission
		r.With(auth.RequirePermission(authz, "player.create")).
			Post("/", h.Create)

		// List — any authenticated user
		r.Get("/", h.List)

		// Get by ID — any authenticated user
		r.Get("/{id}", h.GetByID)

		// Update — requires player.update permission
		r.With(auth.RequirePermission(authz, "player.update")).
			Patch("/{id}", h.Update)

		// Delete (soft) — requires player.delete permission
		r.With(auth.RequirePermission(authz, "player.delete")).
			Delete("/{id}", h.Delete)
	})
}
