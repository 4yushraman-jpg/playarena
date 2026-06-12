package matches

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/notifications"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts all match endpoints under
// /api/v1/organizations/{slug}/matches.
//
// Authorization matrix:
//
//	POST         /           RequireAuth + RequirePermission("match.create")
//	GET          /           RequireAuth
//	GET          /{id}       RequireAuth
//	GET          /{id}/score RequireAuth
//	PATCH        /{id}       RequireAuth + RequirePermission("match.update")
//	DELETE       /{id}       RequireAuth + RequirePermission("match.delete")
func RegisterRoutes(
	r chi.Router,
	pool *pgxpool.Pool,
	cfg *config.Config,
	log *slog.Logger,
	authz *auth.AuthorizationService,
	notifSvc *notifications.Service,
) {
	queries := db.New(pool)
	repo := NewRepository(queries, pool)
	svc := NewService(repo, log, notifSvc)
	h := NewHandler(svc, log)

	r.Route("/api/v1/organizations/{slug}/matches", func(r chi.Router) {
		// All match routes require a valid access token with an org context.
		r.Use(auth.RequireAuth(cfg))
		r.Use(auth.RequireOrgScope())

		// Create — requires match.create permission
		r.With(auth.RequirePermission(authz, "match.create")).
			Post("/", h.Create)

		// List — any authenticated user
		r.Get("/", h.List)

		// Get by ID — any authenticated user
		r.Get("/{id}", h.GetByID)

		// Score — derived from effective event log; any authenticated user
		r.Get("/{id}/score", h.GetScore)

		// Update (including status transitions) — requires match.update permission
		r.With(auth.RequirePermission(authz, "match.update")).
			Patch("/{id}", h.Update)

		// Delete (soft-cancel: status → cancelled) — requires match.delete permission
		r.With(auth.RequirePermission(authz, "match.delete")).
			Delete("/{id}", h.Delete)
	})
}
