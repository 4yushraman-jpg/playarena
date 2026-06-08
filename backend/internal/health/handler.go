package health

import (
	"encoding/json"
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

// Ready is the Kubernetes/Docker readiness probe handler.
// It checks that the database is reachable (required for meaningful work).
// Served on the internal observability port — never the public API port.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	if err := database.Health(r.Context(), h.db); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_ready", "reason": "database_unavailable"}) //nolint:errcheck
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"}) //nolint:errcheck
}

// Live is the Kubernetes/Docker liveness probe handler.
// It always returns 200 — if the process can respond to HTTP it is alive.
// Served on the internal observability port — never the public API port.
func (h *Handler) Live(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "alive"}) //nolint:errcheck
}
