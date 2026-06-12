package tournaments

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/notifications"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/rankings"
)

// RegisterRoutes mounts all tournament endpoints under
// /api/v1/organizations/{slug}/tournaments.
//
// Authorization matrix:
//
//	POST   /                  RequireAuth + RequirePermission("tournament.create")
//	GET    /                  RequireAuth
//	GET    /{id}              RequireAuth
//	GET    /{id}/standings    RequireAuth
//	PATCH  /{id}              RequireAuth + RequirePermission("tournament.update")
//	DELETE /{id}              RequireAuth + RequirePermission("tournament.delete")
func RegisterRoutes(
	r chi.Router,
	pool *pgxpool.Pool,
	cfg *config.Config,
	log *slog.Logger,
	authz *auth.AuthorizationService,
	notifSvc *notifications.Service,
	rankingsRepo *rankings.Repository,
) {
	queries := db.New(pool)
	repo := NewRepository(queries, pool)
	svc := NewService(repo, log, notifSvc, rankingsRepo)
	h := NewHandler(svc, log)

	r.Route("/api/v1/organizations/{slug}/tournaments", func(r chi.Router) {
		// All tournament routes require a valid access token with an org context.
		r.Use(auth.RequireAuth(cfg))
		r.Use(auth.RequireOrgScope())

		// Create — requires tournament.create permission
		r.With(auth.RequirePermission(authz, "tournament.create")).
			Post("/", h.Create)

		// List — any authenticated user
		r.Get("/", h.List)

		// Get by ID — any authenticated user
		r.Get("/{id}", h.GetByID)

		// Standings — derived from snapshotted match scores; any authenticated user
		r.Get("/{id}/standings", h.GetStandings)

		// Update (including status transitions) — requires tournament.update permission
		r.With(auth.RequirePermission(authz, "tournament.update")).
			Patch("/{id}", h.Update)

		// Delete (soft-cancel: status → cancelled) — requires tournament.delete permission
		r.With(auth.RequirePermission(authz, "tournament.delete")).
			Delete("/{id}", h.Delete)
	})
}
