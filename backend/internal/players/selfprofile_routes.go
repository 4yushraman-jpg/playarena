package players

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterMeRoutes mounts the GP-1 global PlayerProfile endpoints. These live
// OUTSIDE the org-scoped tree and are gated by cfg.PlayerPersonaEnabled: when
// the flag is off the routes are not mounted and the runtime behaves exactly as
// before GP-1.
//
// Authorization model: these are self-service identity routes. Access is gated
// by RequireAuth plus a service-layer ownership check (user_id == actor) — NOT
// by RequireOrgScope (which would reject the very player/onboarding tokens that
// must reach these routes). GET /players/{id} is visibility-aware.
//
//	POST  /api/v1/me/player        RequireAuth (any scope; one profile per user)
//	GET   /api/v1/me/player        RequireAuth (returns caller's own profile)
//	PATCH /api/v1/me/player        RequireAuth (owner-only via user_id)
//	GET   /api/v1/players/{id}      RequireAuth (visibility-aware)
func RegisterMeRoutes(
	r chi.Router,
	pool *pgxpool.Pool,
	cfg *config.Config,
	log *slog.Logger,
) {
	if !cfg.PlayerPersonaEnabled {
		return
	}

	queries := db.New(pool)
	repo := NewRepository(queries, pool)
	svc := NewSelfService(repo, log)
	h := NewSelfHandler(svc, log)

	r.Route("/api/v1/me/player", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg))
		r.Post("/", h.CreateOwn)
		r.Get("/", h.GetOwn)
		r.Patch("/", h.UpdateOwn)
	})

	r.Route("/api/v1/players", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg))
		r.Get("/{id}", h.GetByID)
	})
}
