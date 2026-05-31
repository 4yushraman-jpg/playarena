package bootstrap

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// App is the composition root of the PlayArena backend.
// It holds all top-level dependencies and assembles the HTTP handler.
type App struct {
	Config *config.Config
	DB     *pgxpool.Pool
	Log    *slog.Logger
}

// Handler returns the fully-wired HTTP handler for the application.
func (a *App) Handler() http.Handler {
	return NewRouter(a.DB, a.Log, a.Config)
}
