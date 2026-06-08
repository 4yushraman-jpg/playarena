package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/4yushraman-jpg/playarena/internal/health"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/metrics"
)

// newInternalServer constructs the internal observability HTTP server that
// serves /metrics, /ready, /live, and optionally /debug/pprof/*.
// This server MUST NOT be exposed via the public load balancer.
func newInternalServer(cfg *config.Config, reg *metrics.Registry, db *pgxpool.Pool, log *slog.Logger) *http.Server {
	r := chi.NewRouter()

	metricsHandler := promhttp.HandlerFor(reg.Prometheus, promhttp.HandlerOpts{
		Registry: reg.Prometheus,
	})
	r.Handle("/metrics", metricsHandler)

	h := health.New(db)
	r.Get("/ready", h.Ready)
	r.Get("/live", h.Live)

	if cfg.PprofEnabled {
		r.HandleFunc("/debug/pprof/", pprof.Index)
		r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		r.HandleFunc("/debug/pprof/profile", pprof.Profile)
		r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		r.HandleFunc("/debug/pprof/trace", pprof.Trace)
		r.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		r.Handle("/debug/pprof/heap", pprof.Handler("heap"))
		r.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
		r.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
		r.Handle("/debug/pprof/block", pprof.Handler("block"))
		r.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
		log.Info("pprof enabled on internal port")
	}

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.AppInternalPort),
		Handler:           r,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// startDBPoolScraper runs a background goroutine that reads pgxpool.Stat()
// every 15 seconds and publishes the values to Prometheus gauges.
// No DB connection is acquired per scrape — Stat() reads in-memory counters.
func startDBPoolScraper(db *pgxpool.Pool, reg *metrics.Registry, done <-chan struct{}) {
	go func() {
		var prevEmptyAcquire int64
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s := db.Stat()
				reg.DBPoolOpen.Set(float64(s.TotalConns()))
				reg.DBPoolAcquired.Set(float64(s.AcquiredConns()))
				reg.DBPoolIdle.Set(float64(s.IdleConns()))
				reg.DBPoolConstructing.Set(float64(s.ConstructingConns()))
				reg.DBPoolMax.Set(float64(s.MaxConns()))
				cur := s.EmptyAcquireCount()
				if delta := cur - prevEmptyAcquire; delta > 0 {
					reg.DBPoolEmptyAcquire.Add(float64(delta))
					prevEmptyAcquire = cur
				}
			case <-done:
				return
			}
		}
	}()
}

// startOutboxMetricsScraper runs a background goroutine that periodically
// reads outbox depth and dead-letter counts and sets the corresponding gauges.
type outboxScraper interface {
	CountPendingOutboxRows(ctx context.Context) (int64, error)
	CountEmailDeadLetters(ctx context.Context) (int64, error)
}

type webhookDeadLetterScraper interface {
	CountDeadLetters(ctx context.Context) (int64, error)
}

func startOutboxMetricsScraper(
	notifRepo outboxScraper,
	webhookRepo webhookDeadLetterScraper,
	reg *metrics.Registry,
	log *slog.Logger,
	done <-chan struct{},
) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				scrapeOutbox(ctx, notifRepo, webhookRepo, reg, log)
				cancel()
			case <-done:
				return
			}
		}
	}()
}

func scrapeOutbox(
	ctx context.Context,
	notifRepo outboxScraper,
	webhookRepo webhookDeadLetterScraper,
	reg *metrics.Registry,
	log *slog.Logger,
) {
	if n, err := notifRepo.CountPendingOutboxRows(ctx); err != nil {
		log.Error("metrics: count pending outbox rows", slog.Any("error", err))
	} else {
		reg.NotifOutboxPending.Set(float64(n))
	}

	if n, err := notifRepo.CountEmailDeadLetters(ctx); err != nil {
		log.Error("metrics: count email dead letters", slog.Any("error", err))
	} else {
		reg.EmailDeadLetters.Set(float64(n))
	}

	if n, err := webhookRepo.CountDeadLetters(ctx); err != nil {
		log.Error("metrics: count webhook dead letters", slog.Any("error", err))
	} else {
		reg.WebhookDeadLetters.Set(float64(n))
	}
}
