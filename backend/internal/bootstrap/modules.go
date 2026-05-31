package bootstrap

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/health"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// registerModules wires all domain modules into the router.
// Add new modules here as the application grows — one call per domain.
func registerModules(r chi.Router, db *pgxpool.Pool, log *slog.Logger, cfg *config.Config) {
	health.RegisterRoutes(r, db)
	auth.RegisterRoutes(r, db, cfg, log)
}
