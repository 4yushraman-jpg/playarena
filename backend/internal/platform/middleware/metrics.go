package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/4yushraman-jpg/playarena/internal/platform/metrics"
)

// Metrics returns a chi middleware that records HTTP request metrics.
// It captures:
//   - playarena_http_requests_total      (counter)  method × route × status_code
//   - playarena_http_request_duration_seconds (histogram) method × route × status_class
//   - playarena_http_requests_in_flight  (gauge)
//
// Route labels use the chi route pattern (e.g. /api/v1/organizations/{slug}),
// never actual URL values, to avoid cardinality explosion on UUID-based paths.
// Requests that match no registered route are labelled route="unmatched".
func Metrics(reg *metrics.Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reg.HTTPInFlight.Inc()
			defer reg.HTTPInFlight.Dec()

			wrapped := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(wrapped, r)

			status := wrapped.Status()
			if status == 0 {
				status = http.StatusOK
			}

			// Extract chi route pattern *after* the handler runs so sub-router
			// patterns are fully resolved.
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}

			elapsed := time.Since(start).Seconds()
			statusStr := strconv.Itoa(status)
			statusClass := fmt.Sprintf("%dxx", status/100)

			reg.HTTPRequests.WithLabelValues(r.Method, route, statusStr).Inc()
			reg.HTTPDuration.WithLabelValues(r.Method, route, statusClass).Observe(elapsed)
		})
	}
}
