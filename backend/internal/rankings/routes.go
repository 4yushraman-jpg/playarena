package rankings

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// RegisterRoutes mounts all rankings endpoints.
//
// Authorization matrix:
//
//	GET /api/v1/organizations/{slug}/rankings/players   RequireAuth
//	GET /api/v1/organizations/{slug}/rankings/teams     RequireAuth
func RegisterRoutes(
	r chi.Router,
	pool *pgxpool.Pool,
	cfg *config.Config,
	log *slog.Logger,
) {
	queries := db.New(pool)
	repo := NewRepository(queries, pool)
	svc := NewService(repo)
	h := NewHandler(svc, log)

	r.Route("/api/v1/organizations/{slug}/rankings", func(r chi.Router) {
		r.Use(auth.RequireAuth(cfg))
		r.Use(auth.RequireOrgScope())
		r.Get("/players", h.ListPlayerRankings)
		r.Get("/teams", h.ListTeamRankings)
	})
}
