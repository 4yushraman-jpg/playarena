package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/4yushraman-jpg/playarena/internal/platform/metrics"
	"github.com/4yushraman-jpg/playarena/internal/platform/middleware"
)

func TestMetricsMiddleware_RoutePattern(t *testing.T) {
	reg := metrics.New()

	r := chi.NewRouter()
	r.Use(middleware.Metrics(reg))
	r.Get("/api/v1/organizations/{slug}/players/{playerID}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Request with a real UUID-like path value — the label must be the pattern,
	// not the actual path, to avoid cardinality explosion.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/my-org/players/550e8400-e29b-41d4-a716-446655440000", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	mfs, err := reg.Prometheus.Gather()
	if err != nil {
		t.Fatalf("Gather(): %v", err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() != "playarena_http_requests_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "route" {
					if lp.GetValue() == "/api/v1/organizations/{slug}/players/{playerID}" {
						found = true
					}
					// Ensure the raw UUID never appears as a label value.
					if lp.GetValue() == "550e8400-e29b-41d4-a716-446655440000" {
						t.Errorf("UUID appeared as route label — cardinality explosion risk")
					}
				}
			}
		}
	}
	if !found {
		t.Error("chi route pattern not used as route label")
	}
}

func TestMetricsMiddleware_UnmatchedRoute(t *testing.T) {
	reg := metrics.New()

	r := chi.NewRouter()
	r.Use(middleware.Metrics(reg))
	// Register at least one route so chi builds its handler chain and runs
	// middleware for unmatched paths (chi with no routes skips middleware).
	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/unknown/path", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	mfs, _ := reg.Prometheus.Gather()
	for _, mf := range mfs {
		if mf.GetName() != "playarena_http_requests_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "route" && lp.GetValue() == "unmatched" {
					return // pass
				}
			}
		}
	}
	t.Error("unmatched route not labelled as 'unmatched'")
}

func TestMetricsMiddleware_InFlight(t *testing.T) {
	reg := metrics.New()

	r := chi.NewRouter()
	r.Use(middleware.Metrics(reg))
	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// After the request completes, in-flight should be back to 0.
	mfs, _ := reg.Prometheus.Gather()
	for _, mf := range mfs {
		if mf.GetName() == "playarena_http_requests_in_flight" {
			for _, m := range mf.GetMetric() {
				if v := m.GetGauge().GetValue(); v != 0 {
					t.Errorf("in-flight gauge should be 0 after request, got %v", v)
				}
			}
		}
	}
}
