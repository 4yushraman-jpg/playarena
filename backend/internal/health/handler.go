package health

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/4yushraman-jpg/playarena/internal/platform/database"
	"github.com/4yushraman-jpg/playarena/internal/platform/response"
)

// Handler handles HTTP requests for health check endpoints.
type Handler struct {
	db *pgxpool.Pool
}

// New creates a Handler backed by the given connection pool.
func New(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

// Check handles GET /api/v1/health.
//
// Returns 200 with status "ok" when the database is reachable.
// Returns 503 with status "degraded" when the database is unreachable.
func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	dbStatus := "connected"
	httpStatus := http.StatusOK
	appStatus := "ok"

	if err := database.Health(r.Context(), h.db); err != nil {
		dbStatus = "disconnected"
		httpStatus = http.StatusServiceUnavailable
		appStatus = "degraded"
	}

	response.Write(w, httpStatus, healthResponse{
		Status:   appStatus,
		Database: dbStatus,
	})
}
