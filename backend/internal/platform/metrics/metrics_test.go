package metrics_test

import (
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/4yushraman-jpg/playarena/internal/platform/metrics"
)

func TestNew_RegistersWithoutPanic(t *testing.T) {
	// Constructing two separate registries in the same process must not panic
	// (each uses its own prometheus.NewRegistry, not the default global registry).
	r1 := metrics.New()
	r2 := metrics.New()
	if r1 == nil || r2 == nil {
		t.Fatal("New() returned nil")
	}
	if r1.Prometheus == r2.Prometheus {
		t.Error("two Registry instances should have distinct Prometheus registries")
	}
}

func TestNew_MetricsAreRegistered(t *testing.T) {
	reg := metrics.New()

	// Prometheus Gather() only returns metric families with at least one observed
	// value. Force one observation on each vec/counter/gauge so all appear.
	reg.HTTPRequests.WithLabelValues("GET", "/test", "200").Inc()
	reg.HTTPDuration.WithLabelValues("GET", "/test", "2xx").Observe(0.1)
	reg.HTTPInFlight.Set(0)
	reg.RateLimitRejections.WithLabelValues("auth").Inc()
	reg.DBPoolOpen.Set(0)
	reg.AuthLoginTotal.WithLabelValues("success").Inc()
	reg.AuthRefreshTotal.WithLabelValues("success").Inc()
	reg.AuthReplayTotal.Inc()
	reg.AuthPasswordResetTotal.Inc()
	reg.NotifOutboxPending.Set(0)
	reg.NotifDrainDuration.Observe(0)
	reg.NotifDrainTotal.WithLabelValues("success").Inc()
	reg.EmailWorkerTickTotal.WithLabelValues("success").Inc()
	reg.EmailWorkerDeliveryTotal.WithLabelValues("success").Inc()
	reg.EmailWorkerBatchSize.Observe(0)
	reg.EmailDeadLetters.Set(0)
	reg.WebhookWorkerTickTotal.WithLabelValues("success").Inc()
	reg.WebhookWorkerDeliveryTotal.WithLabelValues("success").Inc()
	reg.WebhookWorkerBatchSize.Observe(0)
	reg.WebhookDeadLetters.Set(0)
	reg.RealtimeSubscribers.Set(0)
	reg.RealtimeSubscribeTotal.Inc()
	reg.RealtimeUnsubTotal.Inc()
	reg.RealtimePublishTotal.Inc()
	reg.RealtimeDroppedTotal.Inc()

	mfs, err := reg.Prometheus.Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}

	names := make(map[string]struct{}, len(mfs))
	for _, mf := range mfs {
		names[mf.GetName()] = struct{}{}
	}

	required := []string{
		"playarena_http_requests_total",
		"playarena_http_request_duration_seconds",
		"playarena_http_requests_in_flight",
		"playarena_rate_limit_rejections_total",
		"playarena_db_pool_open_connections",
		"playarena_auth_login_attempts_total",
		"playarena_auth_token_refresh_total",
		"playarena_auth_token_replay_detections_total",
		"playarena_auth_password_reset_requests_total",
		"playarena_notification_outbox_pending_rows",
		"playarena_notification_drain_duration_seconds",
		"playarena_notification_drain_total",
		"playarena_email_worker_tick_total",
		"playarena_email_worker_deliveries_total",
		"playarena_email_worker_batch_size",
		"playarena_email_dead_letters_total",
		"playarena_webhook_worker_tick_total",
		"playarena_webhook_worker_deliveries_total",
		"playarena_webhook_worker_batch_size",
		"playarena_webhook_dead_letters_total",
		"playarena_realtime_active_subscribers",
		"playarena_realtime_subscribe_total",
		"playarena_realtime_unsubscribe_total",
		"playarena_realtime_publish_total",
		"playarena_realtime_dropped_events_total",
	}

	for _, name := range required {
		if _, ok := names[name]; !ok {
			t.Errorf("metric %q not found in Gather() output after recording a value", name)
		}
	}
}

func TestNew_PrefixOnAllMetrics(t *testing.T) {
	reg := metrics.New()
	mfs, err := reg.Prometheus.Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}

	for _, mf := range mfs {
		name := mf.GetName()
		// Skip Go runtime and process metrics from the built-in collectors.
		if strings.HasPrefix(name, "go_") || strings.HasPrefix(name, "process_") {
			continue
		}
		const prefix = "playarena_"
		if len(name) < len(prefix) || name[:len(prefix)] != prefix {
			t.Errorf("metric %q does not have the %q prefix", name, prefix)
		}
	}
}

func TestAuthLoginTotal_RecordsLabels(t *testing.T) {
	reg := metrics.New()
	reg.AuthLoginTotal.WithLabelValues("success").Inc()
	reg.AuthLoginTotal.WithLabelValues("invalid_credentials").Add(3)

	mfs, _ := reg.Prometheus.Gather()
	var loginMF *dto.MetricFamily
	for _, mf := range mfs {
		if mf.GetName() == "playarena_auth_login_attempts_total" {
			loginMF = mf
			break
		}
	}
	if loginMF == nil {
		t.Fatal("playarena_auth_login_attempts_total not found after recording")
	}

	counts := make(map[string]float64)
	for _, m := range loginMF.GetMetric() {
		for _, lp := range m.GetLabel() {
			if lp.GetName() == "result" {
				counts[lp.GetValue()] = m.GetCounter().GetValue()
			}
		}
	}
	if counts["success"] != 1 {
		t.Errorf("expected success=1, got %v", counts["success"])
	}
	if counts["invalid_credentials"] != 3 {
		t.Errorf("expected invalid_credentials=3, got %v", counts["invalid_credentials"])
	}
}
