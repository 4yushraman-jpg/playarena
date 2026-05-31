package health

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterRoutes mounts all health check routes onto r.
func RegisterRoutes(r chi.Router, db *pgxpool.Pool) {
	h := New(db)
	r.Get("/api/v1/health", h.Check)
}
