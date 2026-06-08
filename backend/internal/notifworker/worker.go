// Package notifworker implements the EmailWorker: a background goroutine that
// claims pending email channel notification rows from the database and delivers
// them via the configured email provider.
//
// Delivery guarantees:
//
//	At-least-once: a crash after Send but before RecordSuccess causes a retry.
//	Exactly-once state: RecordSuccess is guarded by sent_at IS NULL so a
//	duplicate success write is a no-op.
//
// Retry schedule (attempt_count is incremented at claim time):
//
//	Attempt 1 fails → retry in 1 minute
//	Attempt 2 fails → retry in 5 minutes
//	Attempt 3 fails → failed_permanently = TRUE (manual intervention required)
//
// Concurrency: multiple EmailWorker instances (or multiple ticks of the same
// instance) are safe — the claim query uses FOR UPDATE SKIP LOCKED so each
// row is processed by at most one worker per tick.
package notifworker

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/email"
	"github.com/4yushraman-jpg/playarena/internal/platform/metrics"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

const (
	maxAttempts int32 = 3
	batchSize   int32 = 10
)

// EmailWorker polls the database for pending email notifications and delivers
// them via the configured Sender. Embedded in the same binary as the API server;
// lifecycle is managed through Start / Stop / Drain.
type EmailWorker struct {
	repo       *Repository
	sender     *email.Sender
	appBaseURL string
	interval   time.Duration
	log        *slog.Logger
	reg        *metrics.Registry // nil means no metrics
	done       chan struct{}
}

// NewEmailWorker constructs an EmailWorker. interval controls the polling
// frequency (e.g., 30 * time.Second). appBaseURL is used to construct
// deep-links in notification emails (e.g., "https://app.playarena.com").
// reg may be nil; metrics recording is skipped when nil.
func NewEmailWorker(
	pool *pgxpool.Pool,
	sender *email.Sender,
	appBaseURL string,
	interval time.Duration,
	log *slog.Logger,
	reg *metrics.Registry,
) *EmailWorker {
	return &EmailWorker{
		repo:       NewRepository(db.New(pool), pool),
		sender:     sender,
		appBaseURL: appBaseURL,
		interval:   interval,
		log:        log,
		reg:        reg,
		done:       make(chan struct{}),
	}
}

// Start launches the polling loop in a background goroutine. Non-blocking.
func (w *EmailWorker) Start() {
	go w.run()
}

// Stop signals the polling loop to exit. Non-blocking; the goroutine may
// still be mid-delivery when Stop returns. Call Drain after Stop for a clean
// graceful shutdown that processes any in-flight batch.
func (w *EmailWorker) Stop() {
	select {
	case <-w.done:
		// already stopped
	default:
		close(w.done)
	}
}

// Drain runs one final delivery pass and returns when all claimed rows have
// been processed. Called by App.Shutdown after Stop to flush the last batch
// before the process exits.
func (w *EmailWorker) Drain(ctx context.Context) error {
	return w.runOnce(ctx)
}

func (w *EmailWorker) run() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx := context.Background()
			if err := w.runOnce(ctx); err != nil {
				w.log.Error("notifworker: tick error", slog.Any("error", err))
			}
		case <-w.done:
			return
		}
	}
}

func (w *EmailWorker) runOnce(ctx context.Context) error {
	rows, err := w.repo.ClaimBatch(ctx, maxAttempts, batchSize)
	if err != nil {
		if w.reg != nil {
			w.reg.EmailWorkerTickTotal.WithLabelValues("error").Inc()
		}
		return err
	}
	if w.reg != nil {
		w.reg.EmailWorkerBatchSize.Observe(float64(len(rows)))
		w.reg.EmailWorkerTickTotal.WithLabelValues("success").Inc()
	}
	for _, row := range rows {
		w.deliver(ctx, row)
	}
	return nil
}

func (w *EmailWorker) deliver(ctx context.Context, n db.Notification) {
	nid := pgutil.UUIDToString(n.ID)

	user, err := w.repo.GetUserByID(ctx, n.UserID)
	if err != nil {
		w.log.Error("notifworker: get user",
			slog.String("notification_id", nid),
			slog.Any("error", err),
		)
		w.recordFailure(ctx, n)
		return
	}

	orgSlug, err := w.repo.GetOrgSlugByID(ctx, n.OrganizationID)
	if err != nil {
		w.log.Error("notifworker: get org slug",
			slog.String("notification_id", nid),
			slog.Any("error", err),
		)
		w.recordFailure(ctx, n)
		return
	}

	notificationsURL := w.appBaseURL + "/organizations/" + orgSlug + "/notifications"
	displayName := strings.TrimSpace(user.FirstName + " " + user.LastName)
	label := eventLabel(n.EventType)

	if err := w.sender.SendNotificationEmail(ctx, user.Email, displayName, label, notificationsURL); err != nil {
		w.log.Error("notifworker: send failed",
			slog.String("notification_id", nid),
			slog.String("to", user.Email),
			slog.String("event_type", string(n.EventType)),
			slog.Any("error", err),
		)
		w.recordFailure(ctx, n)
		return
	}

	if err := w.repo.RecordSuccess(ctx, n.ID); err != nil {
		w.log.Error("notifworker: record success",
			slog.String("notification_id", nid),
			slog.Any("error", err),
		)
		return
	}

	if w.reg != nil {
		w.reg.EmailWorkerDeliveryTotal.WithLabelValues("success").Inc()
	}

	w.log.Info("notifworker: delivered",
		slog.String("notification_id", nid),
		slog.String("to", user.Email),
		slog.String("event_type", string(n.EventType)),
		slog.Int("attempt_count", int(n.AttemptCount)),
	)
}

func (w *EmailWorker) recordFailure(ctx context.Context, n db.Notification) {
	perm := n.AttemptCount >= maxAttempts
	var nextAt pgtype.Timestamptz
	if !perm {
		nextAt = pgtype.Timestamptz{
			Time:  time.Now().UTC().Add(retryDelay(n.AttemptCount)),
			Valid: true,
		}
	}
	if err := w.repo.RecordFailure(ctx, n.ID, perm, nextAt); err != nil {
		w.log.Error("notifworker: record failure",
			slog.String("notification_id", pgutil.UUIDToString(n.ID)),
			slog.Any("error", err),
		)
	}
	if w.reg != nil {
		status := "failure"
		if perm {
			status = "permanent_failure"
		}
		w.reg.EmailWorkerDeliveryTotal.WithLabelValues(status).Inc()
	}
	if perm {
		w.log.Warn("notifworker: permanently failed",
			slog.String("notification_id", pgutil.UUIDToString(n.ID)),
			slog.Int("attempt_count", int(n.AttemptCount)),
		)
	}
}

// retryDelay returns the wait before the next delivery attempt.
// attempt_count is already incremented at claim time:
//
//	1 → retry in 1 minute
//	2 → retry in 5 minutes
func retryDelay(attemptCount int32) time.Duration {
	switch attemptCount {
	case 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	default:
		return 15 * time.Minute
	}
}

// eventLabel converts a NotificationEventType to a human-readable label
// suitable for use in email subjects and body text.
func eventLabel(et db.NotificationEventType) string {
	switch et {
	case db.NotificationEventTypeMatchCreated:
		return "Match Created"
	case db.NotificationEventTypeMatchStarted:
		return "Match Started"
	case db.NotificationEventTypeMatchCompleted:
		return "Match Completed"
	case db.NotificationEventTypeMatchCancelled:
		return "Match Cancelled"
	case db.NotificationEventTypeMatchAbandoned:
		return "Match Abandoned"
	case db.NotificationEventTypeTournamentStatusChanged:
		return "Tournament Status Changed"
	case db.NotificationEventTypeRegistrationApproved:
		return "Registration Approved"
	case db.NotificationEventTypeRegistrationRejected:
		return "Registration Rejected"
	case db.NotificationEventTypeRegistrationWithdrawn:
		return "Registration Withdrawn"
	default:
		return string(et)
	}
}
