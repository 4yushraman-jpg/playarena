package tournament_registrations

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/notifications"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts all tournament registration endpoints under
// /api/v1/organizations/{slug}/tournaments/{tournamentId}/registrations.
//
// Authorization matrix:
//
//	POST   /                        RequireAuth + RequirePermission("tournament.update")
//	GET    /                        RequireAuth
//	GET    /{registrationId}        RequireAuth
//	PATCH  /{registrationId}        RequireAuth + RequirePermission("tournament.update")
//	DELETE /{registrationId}        RequireAuth + RequirePermission("tournament.update")
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

	r.Route("/api/v1/organizations/{slug}/tournaments/{tournamentId}/registrations", func(r chi.Router) {
		// All registration routes require a valid access token.
		r.Use(auth.RequireAuth(cfg))

		// Register (create) — requires tournament.update permission
		r.With(auth.RequirePermission(authz, "tournament.update")).
			Post("/", h.Register)

		// List — any authenticated user
		r.Get("/", h.List)

		// Get by ID — any authenticated user
		r.Get("/{registrationId}", h.GetByID)

		// Update (status transitions, notes, seed) — requires tournament.update
		r.With(auth.RequirePermission(authz, "tournament.update")).
			Patch("/{registrationId}", h.Update)

		// Delete (soft-withdraw: status → withdrawn) — requires tournament.update
		r.With(auth.RequirePermission(authz, "tournament.update")).
			Delete("/{registrationId}", h.Delete)
	})
}
