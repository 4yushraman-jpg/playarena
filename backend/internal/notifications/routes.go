package notifications

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts all notification endpoints under
// /api/v1/organizations/{slug}/notifications.
//
// Authorization matrix (RequireAuth only — no RBAC on personal endpoints):
//
//	GET    /                       RequireAuth — list caller's notifications
//	GET    /{id}                   RequireAuth — get single notification
//	PATCH  /{id}/read              RequireAuth — mark single notification read
//	POST   /read-all               RequireAuth — mark all notifications read
//	DELETE /{id}                   RequireAuth — soft-delete a notification
//	GET    /preferences            RequireAuth — get caller's preferences
//	PUT    /preferences/{event_type} RequireAuth — upsert a preference
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

	r.Route("/api/v1/organizations/{slug}/notifications", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg))

		// Preferences sub-routes — must be registered before /{id} to avoid
		// chi routing "preferences" as a UUID param.
		r.Get("/preferences", h.GetPreferences)
		r.Put("/preferences/{event_type}", h.UpdatePreference)

		// Read-all — registered before /{id}/read to avoid routing conflict.
		r.Post("/read-all", h.MarkAllRead)

		// Notification CRUD.
		r.Get("/", h.List)
		r.Get("/{id}", h.GetByID)
		r.Patch("/{id}/read", h.MarkRead)
		r.Delete("/{id}", h.Delete)
	})
}
