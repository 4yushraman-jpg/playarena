package webhooks

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts webhook endpoints under /api/v1/organizations/{slug}/webhooks.
//
//	POST   /{slug}/webhooks                        webhook.create
//	GET    /{slug}/webhooks                        webhook.read
//	GET    /{slug}/webhooks/{webhookID}            webhook.read
//	PATCH  /{slug}/webhooks/{webhookID}/active     webhook.update
//	DELETE /{slug}/webhooks/{webhookID}            webhook.delete
func RegisterRoutes(
	r chi.Router,
	pool *pgxpool.Pool,
	cfg *config.Config,
	log *slog.Logger,
	authz *auth.AuthorizationService,
) {
	queries := db.New(pool)
	repo := NewRepository(queries, pool)

	svc, err := NewService(repo, cfg.WebhookSecretKey, log)
	if err != nil {
		log.Error("bootstrap: failed to initialise webhook service",
			slog.String("error", err.Error()),
		)
		panic("webhook service initialisation failed: " + err.Error())
	}

	h := NewHandler(svc, log)

	r.Route("/api/v1/organizations/{slug}/webhooks", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg))

		r.With(auth.RequirePermission(authz, "webhook.create")).
			Post("/", h.Create)

		r.With(auth.RequirePermission(authz, "webhook.read")).
			Get("/", h.List)

		r.With(auth.RequirePermission(authz, "webhook.read")).
			Get("/{webhookID}", h.GetByID)

		r.With(auth.RequirePermission(authz, "webhook.update")).
			Patch("/{webhookID}/active", h.UpdateActive)

		r.With(auth.RequirePermission(authz, "webhook.delete")).
			Delete("/{webhookID}", h.Delete)
	})
}
