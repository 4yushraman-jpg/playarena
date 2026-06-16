package public

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/middleware"
)

// RegisterRoutes mounts the anonymous public read API under /api/v1/public.
//
// CRITICAL: this tree carries NO auth middleware and NO org-scope middleware. It
// is mounted OUTSIDE the authenticated route groups. Authorization is enforced
// exclusively by the SQL visibility gate in the queries — never by a principal
// check (there is no principal). A dedicated per-IP rate limiter throttles
// scraping; responses are GET-only and cacheable.
func RegisterRoutes(
	r chi.Router,
	pool *pgxpool.Pool,
	log *slog.Logger,
	limiter *middleware.IPRateLimiter,
) {
	queries := db.New(pool)
	repo := NewRepository(queries)
	svc := NewService(repo)
	h := NewHandler(svc, log)

	r.Route("/api/v1/public/orgs/{orgSlug}/tournaments/{tournamentSlug}", func(r chi.Router) {
		if limiter != nil {
			r.Use(limiter.Middleware())
		}
		r.Get("/", h.Tournament)
		r.Get("/matches", h.Matches)
		r.Get("/standings", h.Standings)
		r.Get("/matches/{matchId}", h.Match)
	})
}
