package fixturegen

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts fixture-generation endpoints under
// /api/v1/organizations/{slug}/tournaments/{id}/fixtures.
//
// Authorization matrix:
//
//	POST /generate            RequireAuth + RequirePermission("tournament.update")
//	POST /resolve-qualifiers  RequireAuth + RequirePermission("tournament.update")
//	GET  /generation          RequireAuth
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

	r.Route("/api/v1/organizations/{slug}/tournaments/{id}/fixtures", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg))
		r.Use(auth.RequireOrgScope())

		r.With(auth.RequirePermission(authz, "tournament.update")).
			Post("/generate", h.Generate)
		r.With(auth.RequirePermission(authz, "tournament.update")).
			Post("/resolve-qualifiers", h.ResolveQualifiers)
		r.Get("/generation", h.GenerationInfo)
	})
}
