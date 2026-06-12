package members

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts all member management endpoints under
// /api/v1/organizations/{slug}/members.
//
// Authorization matrix:
//
//	GET    /                          RequireAuth + RequirePermission("role.assign")
//	GET    /{userID}                  RequireAuth + RequirePermission("role.assign")
//	POST   /{userID}/roles            RequireAuth + RequirePermission("role.assign")
//	DELETE /{userID}/roles/{roleSlug} RequireAuth + RequirePermission("role.assign")
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

	r.Route("/api/v1/organizations/{slug}/members", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg))
		r.Use(auth.RequireOrgScope())

		r.With(auth.RequirePermission(authz, "role.assign")).Get("/", h.List)
		r.With(auth.RequirePermission(authz, "role.assign")).Get("/{userID}", h.GetMember)
		r.With(auth.RequirePermission(authz, "role.assign")).Post("/{userID}/roles", h.GrantRole)
		r.With(auth.RequirePermission(authz, "role.assign")).Delete("/{userID}/roles/{roleSlug}", h.RevokeRole)
	})
}
