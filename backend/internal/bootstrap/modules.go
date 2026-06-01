package bootstrap

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/health"
	"github.com/4yushraman-jpg/playarena/internal/organizations"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/players"
	"github.com/4yushraman-jpg/playarena/internal/teams"
	"github.com/4yushraman-jpg/playarena/internal/tournament_registrations"
	"github.com/4yushraman-jpg/playarena/internal/tournaments"
)

// registerModules wires all domain modules into the router.
// Add new modules here as the application grows — one call per domain.
func registerModules(r chi.Router, pool *pgxpool.Pool, log *slog.Logger, cfg *config.Config) {
	// AuthorizationService is constructed once and shared across all modules
	// that need permission checks. It is cheap to create (wraps a *db.Queries).
	queries := db.New(pool)
	authz := auth.NewAuthorizationService(queries)

	health.RegisterRoutes(r, pool)
	auth.RegisterRoutes(r, pool, cfg, log)
	organizations.RegisterRoutes(r, pool, cfg, log, authz)
	players.RegisterRoutes(r, pool, cfg, log, authz)
	teams.RegisterRoutes(r, pool, cfg, log, authz)
	tournaments.RegisterRoutes(r, pool, cfg, log, authz)
	tournament_registrations.RegisterRoutes(r, pool, cfg, log, authz)
}
