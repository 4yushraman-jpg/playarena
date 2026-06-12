package teams

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts all team and team-membership endpoints under
// /api/v1/organizations/{slug}/teams.
//
// Authorization matrix:
//
//	POST   /                        RequireAuth + RequirePermission("team.create")
//	GET    /                        RequireAuth
//	GET    /{id}                    RequireAuth
//	PATCH  /{id}                    RequireAuth + RequirePermission("team.update")
//	DELETE /{id}                    RequireAuth + RequirePermission("team.delete")
//
//	POST   /{id}/members            RequireAuth + RequirePermission("team.update")
//	GET    /{id}/members            RequireAuth
//	DELETE /{id}/members/{membershipId}  RequireAuth + RequirePermission("team.update")
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

	r.Route("/api/v1/organizations/{slug}/teams", func(r chi.Router) {
		// All team routes require a valid access token with an org context.
		r.Use(auth.RequireAuth(cfg))
		r.Use(auth.RequireOrgScope())

		// ── team CRUD ────────────────────────────────────────────────────────

		// Create — requires team.create permission
		r.With(auth.RequirePermission(authz, "team.create")).
			Post("/", h.Create)

		// List — any authenticated user
		r.Get("/", h.List)

		// Get by ID — any authenticated user
		r.Get("/{id}", h.GetByID)

		// Update — requires team.update permission
		r.With(auth.RequirePermission(authz, "team.update")).
			Patch("/{id}", h.Update)

		// Delete (soft: status → disbanded) — requires team.delete permission
		r.With(auth.RequirePermission(authz, "team.delete")).
			Delete("/{id}", h.Delete)

		// ── team memberships ─────────────────────────────────────────────────

		// Add member — requires team.update permission (roster management)
		r.With(auth.RequirePermission(authz, "team.update")).
			Post("/{id}/members", h.AddMember)

		// List active members — any authenticated user
		r.Get("/{id}/members", h.ListMembers)

		// Remove member (soft: status → released, left_at = NOW()) — requires team.update
		r.With(auth.RequirePermission(authz, "team.update")).
			Delete("/{id}/members/{membershipId}", h.RemoveMember)
	})
}
