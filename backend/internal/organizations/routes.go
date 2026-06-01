package organizations

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts all organization endpoints onto r under /api/v1/organizations.
//
// All routes require a valid access token (RequireAuth).
//
// Additional permission requirements:
//
//	POST   /                  organization.create
//	GET    /                  (auth only — any authenticated user)
//	GET    /{slug}            (auth only — any authenticated user)
//	PATCH  /{slug}            organization.update
//	DELETE /{slug}            organization.delete
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

	r.Route("/api/v1/organizations", func(r chi.Router) {
		// All organization routes require a valid access token.
		r.Use(auth.RequireAuth(cfg))

		// Create — requires organization.create permission
		r.With(auth.RequirePermission(authz, "organization.create")).
			Post("/", h.Create)

		// List — any authenticated user
		r.Get("/", h.List)

		// Get by slug — any authenticated user
		r.Get("/{slug}", h.GetBySlug)

		// Update — requires organization.update permission
		r.With(auth.RequirePermission(authz, "organization.update")).
			Patch("/{slug}", h.Update)

		// Delete — requires organization.delete permission
		r.With(auth.RequirePermission(authz, "organization.delete")).
			Delete("/{slug}", h.Delete)
	})
}
