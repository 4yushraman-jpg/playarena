package match_events

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts all match event endpoints under
// /api/v1/organizations/{slug}/matches/{matchId}/events.
//
// Authorization matrix:
//
//	POST   /            RequireAuth + RequirePermission("match.score")
//	GET    /            RequireAuth
//	GET    /{eventId}   RequireAuth
//
// No PATCH or DELETE: match_events is append-only by design.
// Corrections are represented by inserting a score_correction event.
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

	r.Route("/api/v1/organizations/{slug}/matches/{matchId}/events", func(r chi.Router) {
		// All event routes require a valid access token with an org context.
		r.Use(auth.RequireAuth(cfg))
		r.Use(auth.RequireOrgScope())

		// Create — requires match.score permission
		r.With(auth.RequirePermission(authz, "match.score")).
			Post("/", h.Create)

		// List — any authenticated user; supports ?effective_only=true
		r.Get("/", h.List)

		// Get by ID — any authenticated user
		r.Get("/{eventId}", h.GetByID)
	})
}
