package bootstrap

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/health"
	"github.com/4yushraman-jpg/playarena/internal/match_events"
	"github.com/4yushraman-jpg/playarena/internal/matches"
	"github.com/4yushraman-jpg/playarena/internal/media"
	mediastorage "github.com/4yushraman-jpg/playarena/internal/media/storage"
	"github.com/4yushraman-jpg/playarena/internal/notifications"
	"github.com/4yushraman-jpg/playarena/internal/organizations"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/middleware"
	"github.com/4yushraman-jpg/playarena/internal/players"
	"github.com/4yushraman-jpg/playarena/internal/teams"
	"github.com/4yushraman-jpg/playarena/internal/tournament_registrations"
	"github.com/4yushraman-jpg/playarena/internal/tournaments"
)

// registerModules wires all domain modules into the router.
func registerModules(
	r chi.Router,
	pool *pgxpool.Pool,
	log *slog.Logger,
	cfg *config.Config,
	limiter *middleware.IPRateLimiter,
) {
	queries := db.New(pool)
	authz := auth.NewAuthorizationService(queries)

	notifRepo := notifications.NewRepository(queries, pool)
	notifSvc := notifications.NewService(notifRepo, log)

	health.RegisterRoutes(r, pool)
	auth.RegisterRoutes(r, pool, cfg, log, limiter)
	organizations.RegisterRoutes(r, pool, cfg, log, authz)
	players.RegisterRoutes(r, pool, cfg, log, authz)
	teams.RegisterRoutes(r, pool, cfg, log, authz)
	tournaments.RegisterRoutes(r, pool, cfg, log, authz, notifSvc)
	tournament_registrations.RegisterRoutes(r, pool, cfg, log, authz, notifSvc)
	matches.RegisterRoutes(r, pool, cfg, log, authz, notifSvc)
	match_events.RegisterRoutes(r, pool, cfg, log, authz)
	notifications.RegisterRoutes(r, pool, cfg, log, authz)

	mediaBackend, err := mediastorage.New(cfg)
	if err != nil {
		log.Error("bootstrap: failed to initialise media storage backend",
			slog.String("backend", cfg.StorageBackend),
			slog.Any("error", err),
		)
		panic("media storage backend initialisation failed: " + err.Error())
	}
	media.RegisterRoutes(r, pool, cfg, log, authz, mediaBackend)
}
