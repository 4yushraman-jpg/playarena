// Package metrics defines the Prometheus metrics registry for PlayArena.
// All metrics use the "playarena_" prefix and follow Prometheus naming conventions.
// A single Registry is constructed at startup and injected into components that
// produce measurements.
//
// Cardinality rules:
//   - Route labels use chi route patterns (e.g. /api/v1/organizations/{slug}),
//     never actual URL values.
//   - Worker, result, and channel labels are drawn from small, bounded enums.
//   - user_id, org_id, request_id, delivery_id are never used as labels.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Registry holds all Prometheus metric descriptors for the application.
// All fields are nil-safe at call sites: components check reg != nil before
// recording. The zero value is therefore never used; always construct via New().
type Registry struct {
	// Prometheus is the underlying registry. Use it to create the /metrics handler.
	Prometheus *prometheus.Registry

	// ── HTTP ──────────────────────────────────────────────────────────────────
	HTTPRequests *prometheus.CounterVec   // labels: method, route, status_code
	HTTPDuration *prometheus.HistogramVec // labels: method, route, status_class
	HTTPInFlight prometheus.Gauge

	// ── Rate limiting ──────────────────────────────────────────────────────────
	RateLimitRejections *prometheus.CounterVec // labels: limiter

	// ── DB pool (updated by background scraper, no per-request DB access) ─────
	DBPoolOpen         prometheus.Gauge
	DBPoolAcquired     prometheus.Gauge
	DBPoolIdle         prometheus.Gauge
	DBPoolConstructing prometheus.Gauge
	DBPoolMax          prometheus.Gauge
	DBPoolEmptyAcquire prometheus.Counter

	// ── Auth ──────────────────────────────────────────────────────────────────
	AuthLoginTotal         *prometheus.CounterVec // labels: result
	AuthRefreshTotal       *prometheus.CounterVec // labels: result
	AuthReplayTotal        prometheus.Counter
	AuthPasswordResetTotal prometheus.Counter

	// ── Notifications ─────────────────────────────────────────────────────────
	NotifOutboxPending prometheus.Gauge
	NotifDrainDuration prometheus.Histogram
	NotifDrainTotal    *prometheus.CounterVec // labels: result

	// ── Email worker ──────────────────────────────────────────────────────────
	EmailWorkerTickTotal     *prometheus.CounterVec // labels: result
	EmailWorkerDeliveryTotal *prometheus.CounterVec // labels: status
	EmailWorkerBatchSize     prometheus.Histogram
	EmailDeadLetters         prometheus.Gauge

	// ── Webhook worker ────────────────────────────────────────────────────────
	WebhookWorkerTickTotal     *prometheus.CounterVec // labels: result
	WebhookWorkerDeliveryTotal *prometheus.CounterVec // labels: status
	WebhookWorkerBatchSize     prometheus.Histogram
	WebhookDeadLetters         prometheus.Gauge

	// ── Realtime hub ──────────────────────────────────────────────────────────
	RealtimeSubscribers    prometheus.Gauge
	RealtimeSubscribeTotal prometheus.Counter
	RealtimeUnsubTotal     prometheus.Counter
	RealtimePublishTotal   prometheus.Counter
	RealtimeDroppedTotal   prometheus.Counter
}

// New constructs a Registry with all metrics registered against a fresh
// Prometheus registry that also includes the default Go runtime and process
// collectors.
func New() *Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	r := &Registry{
		Prometheus: reg,

		// HTTP
		HTTPRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "playarena_http_requests_total",
			Help: "Total HTTP requests by method, route, and status code.",
		}, []string{"method", "route", "status_code"}),
		HTTPDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "playarena_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: []float64{.005, .025, .05, .1, .25, .5, 1, 2.5, 5},
		}, []string{"method", "route", "status_class"}),
		HTTPInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "playarena_http_requests_in_flight",
			Help: "Number of HTTP requests currently being processed.",
		}),

		// Rate limiting
		RateLimitRejections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "playarena_rate_limit_rejections_total",
			Help: "Total requests rejected by the per-IP rate limiter, by limiter type.",
		}, []string{"limiter"}),

		// DB pool
		DBPoolOpen: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "playarena_db_pool_open_connections",
			Help: "Current number of open database connections.",
		}),
		DBPoolAcquired: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "playarena_db_pool_acquired_connections",
			Help: "Current number of acquired (in-use) database connections.",
		}),
		DBPoolIdle: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "playarena_db_pool_idle_connections",
			Help: "Current number of idle database connections.",
		}),
		DBPoolConstructing: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "playarena_db_pool_constructing_connections",
			Help: "Current number of database connections being established.",
		}),
		DBPoolMax: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "playarena_db_pool_max_connections",
			Help: "Configured maximum number of database connections.",
		}),
		DBPoolEmptyAcquire: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "playarena_db_pool_empty_acquire_total",
			Help: "Total number of times an acquire was blocked because the pool was empty.",
		}),

		// Auth
		AuthLoginTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "playarena_auth_login_attempts_total",
			Help: "Total login attempts by result.",
		}, []string{"result"}),
		AuthRefreshTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "playarena_auth_token_refresh_total",
			Help: "Total token refresh attempts by result.",
		}, []string{"result"}),
		AuthReplayTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "playarena_auth_token_replay_detections_total",
			Help: "Total refresh token replay detections (ErrTokenReuse events).",
		}),
		AuthPasswordResetTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "playarena_auth_password_reset_requests_total",
			Help: "Total forgot-password requests.",
		}),

		// Notifications
		NotifOutboxPending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "playarena_notification_outbox_pending_rows",
			Help: "Current number of unprocessed notification outbox rows.",
		}),
		NotifDrainDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "playarena_notification_drain_duration_seconds",
			Help:    "Duration of DrainOutbox calls in seconds.",
			Buckets: []float64{.001, .005, .01, .05, .1, .25, .5, 1, 2.5, 5},
		}),
		NotifDrainTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "playarena_notification_drain_total",
			Help: "Total DrainOutbox calls by result.",
		}, []string{"result"}),

		// Email worker
		EmailWorkerTickTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "playarena_email_worker_tick_total",
			Help: "Total email worker polling ticks by result.",
		}, []string{"result"}),
		EmailWorkerDeliveryTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "playarena_email_worker_deliveries_total",
			Help: "Total email delivery attempts by status.",
		}, []string{"status"}),
		EmailWorkerBatchSize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "playarena_email_worker_batch_size",
			Help:    "Number of email rows claimed per worker tick.",
			Buckets: []float64{0, 1, 2, 5, 10, 20, 50},
		}),
		EmailDeadLetters: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "playarena_email_dead_letters_total",
			Help: "Current number of permanently failed email notification rows.",
		}),

		// Webhook worker
		WebhookWorkerTickTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "playarena_webhook_worker_tick_total",
			Help: "Total webhook worker polling ticks by result.",
		}, []string{"result"}),
		WebhookWorkerDeliveryTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "playarena_webhook_worker_deliveries_total",
			Help: "Total webhook delivery attempts by status.",
		}, []string{"status"}),
		WebhookWorkerBatchSize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "playarena_webhook_worker_batch_size",
			Help:    "Number of webhook delivery rows claimed per worker tick.",
			Buckets: []float64{0, 1, 2, 5, 10, 20, 50},
		}),
		WebhookDeadLetters: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "playarena_webhook_dead_letters_total",
			Help: "Current number of permanently failed webhook delivery rows.",
		}),

		// Realtime hub
		RealtimeSubscribers: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "playarena_realtime_active_subscribers",
			Help: "Current number of active SSE subscriber connections.",
		}),
		RealtimeSubscribeTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "playarena_realtime_subscribe_total",
			Help: "Total SSE subscribe calls.",
		}),
		RealtimeUnsubTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "playarena_realtime_unsubscribe_total",
			Help: "Total SSE unsubscribe calls.",
		}),
		RealtimePublishTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "playarena_realtime_publish_total",
			Help: "Total SSE publish calls.",
		}),
		RealtimeDroppedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "playarena_realtime_dropped_events_total",
			Help: "Total SSE events dropped due to full subscriber channel buffers.",
		}),
	}

	// Register all collectors with the custom registry.
	reg.MustRegister(
		r.HTTPRequests,
		r.HTTPDuration,
		r.HTTPInFlight,
		r.RateLimitRejections,
		r.DBPoolOpen,
		r.DBPoolAcquired,
		r.DBPoolIdle,
		r.DBPoolConstructing,
		r.DBPoolMax,
		r.DBPoolEmptyAcquire,
		r.AuthLoginTotal,
		r.AuthRefreshTotal,
		r.AuthReplayTotal,
		r.AuthPasswordResetTotal,
		r.NotifOutboxPending,
		r.NotifDrainDuration,
		r.NotifDrainTotal,
		r.EmailWorkerTickTotal,
		r.EmailWorkerDeliveryTotal,
		r.EmailWorkerBatchSize,
		r.EmailDeadLetters,
		r.WebhookWorkerTickTotal,
		r.WebhookWorkerDeliveryTotal,
		r.WebhookWorkerBatchSize,
		r.WebhookDeadLetters,
		r.RealtimeSubscribers,
		r.RealtimeSubscribeTotal,
		r.RealtimeUnsubTotal,
		r.RealtimePublishTotal,
		r.RealtimeDroppedTotal,
	)

	return r
}
