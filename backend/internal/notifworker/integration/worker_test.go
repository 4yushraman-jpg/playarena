package notifworker_integration_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/email"
	"github.com/4yushraman-jpg/playarena/internal/notifworker"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── helpers ────────────────────────────────────────────────────────────────────

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testCfg() *config.Config {
	return &config.Config{
		AppEnv:           "development",
		AppBaseURL:       "http://localhost:8080",
		DatabaseURL:      "postgres://integration-test:placeholder/playarena_test",
		JWTSecret:        "test-jwt-secret-key-long-enough!!",
		EmailFromAddress: "noreply@test.example.com",
		EmailFromName:    "PlayArena Test",
	}
}

// seedEmailNotification inserts an outbox row + an email channel notification
// row directly via SQL (fixture pattern — bypasses DrainOutbox since that is
// covered by the notifications integration tests). Returns the email row.
func seedEmailNotification(
	t testing.TB,
	ctx context.Context,
	pool *pgxpool.Pool,
	orgID, userID pgtype.UUID,
) db.Notification {
	t.Helper()

	// Insert outbox entry (required as FK target for the notification row).
	var outboxID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO notification_outbox (organization_id, event_type, entity_type, entity_id, payload)
		VALUES ($1, 'registration_approved', 'tournament_registrations', $1, '{}')
		RETURNING id
	`, orgID).Scan(&outboxID); err != nil {
		t.Fatalf("seedEmailNotification: insert outbox: %v", err)
	}

	// Insert email notification row directly — sent_at = NULL (pending delivery).
	var n db.Notification
	err := pool.QueryRow(ctx, `
		INSERT INTO notifications (organization_id, user_id, outbox_id, channel,
		                           event_type, entity_type, entity_id, payload)
		VALUES ($1, $2, $3, 'email', 'registration_approved', 'tournament_registrations', $1, '{}')
		RETURNING id, organization_id, user_id, outbox_id, channel, event_type,
		          entity_type, entity_id, payload, read_at, sent_at, deleted_at,
		          created_at, attempt_count, last_attempted_at, lease_expires_at,
		          failed_permanently
	`, orgID, userID, outboxID).Scan(
		&n.ID, &n.OrganizationID, &n.UserID, &n.OutboxID, &n.Channel, &n.EventType,
		&n.EntityType, &n.EntityID, &n.Payload, &n.ReadAt, &n.SentAt, &n.DeletedAt,
		&n.CreatedAt, &n.AttemptCount, &n.LastAttemptedAt, &n.LeaseExpiresAt,
		&n.FailedPermanently,
	)
	if err != nil {
		t.Fatalf("seedEmailNotification: insert email notification: %v", err)
	}
	return n
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestEmailWorker_Deliver_Success verifies that the worker claims a pending
// email notification, delivers it via the NoOpProvider, and records sent_at.
func TestEmailWorker_Deliver_Success(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	mailer := &email.NoOpProvider{}
	cfg := testCfg()
	sender := email.NewSenderWithProvider(mailer, email.SenderConfig{
		FromAddress: cfg.EmailFromAddress,
		FromName:    cfg.EmailFromName,
		AppBaseURL:  cfg.AppBaseURL,
	}, discardLog())

	_ = seedEmailNotification(t, ctx, testPool, org.ID, user.ID)

	worker := notifworker.NewEmailWorker(testPool, sender, cfg.AppBaseURL, time.Minute, discardLog())
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	// Email must have been delivered to the user.
	msgs := mailer.SentTo(user.Email)
	if len(msgs) != 1 {
		t.Errorf("sent to %s = %d, want 1", user.Email, len(msgs))
	}

	// sent_at must be set on the notification row.
	var sentAt pgtype.Timestamptz
	if err := testPool.QueryRow(ctx,
		"SELECT sent_at FROM notifications WHERE organization_id = $1 AND user_id = $2 AND channel = 'email'",
		org.ID, user.ID).Scan(&sentAt); err != nil {
		t.Fatalf("select sent_at: %v", err)
	}
	if !sentAt.Valid {
		t.Error("sent_at is NULL after successful delivery, want non-NULL")
	}
}

// TestEmailWorker_Deliver_Idempotent verifies that Drain called twice on the
// same notification does not double-deliver: after RecordSuccess sets sent_at,
// the claim query skips the row on the second call.
func TestEmailWorker_Deliver_Idempotent(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	mailer := &email.NoOpProvider{}
	cfg := testCfg()
	sender := email.NewSenderWithProvider(mailer, email.SenderConfig{
		FromAddress: cfg.EmailFromAddress,
		FromName:    cfg.EmailFromName,
		AppBaseURL:  cfg.AppBaseURL,
	}, discardLog())

	_ = seedEmailNotification(t, ctx, testPool, org.ID, user.ID)

	worker := notifworker.NewEmailWorker(testPool, sender, cfg.AppBaseURL, time.Minute, discardLog())

	// First drain delivers.
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("first Drain: %v", err)
	}
	// Second drain should find no pending rows.
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("second Drain: %v", err)
	}

	msgs := mailer.SentTo(user.Email)
	if len(msgs) != 1 {
		t.Errorf("message count = %d, want 1 (idempotent delivery)", len(msgs))
	}
}

// TestEmailWorker_RetryOnFailure verifies that when delivery fails the row is
// not marked sent_at, attempt_count is already incremented (at claim), and
// failed_permanently stays false until max_attempts is exhausted.
func TestEmailWorker_RetryOnFailure(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	// FailOnce provider fails the first delivery attempt then succeeds.
	failCount := 0
	failOnce := &failingProvider{failN: 1, failCount: &failCount}
	cfg := testCfg()
	sender := email.NewSenderWithProvider(failOnce, email.SenderConfig{
		FromAddress: cfg.EmailFromAddress,
		FromName:    cfg.EmailFromName,
		AppBaseURL:  cfg.AppBaseURL,
	}, discardLog())

	_ = seedEmailNotification(t, ctx, testPool, org.ID, user.ID)

	worker := notifworker.NewEmailWorker(testPool, sender, cfg.AppBaseURL, time.Minute, discardLog())

	// First drain: delivery fails. Claim increments attempt_count to 1.
	// RecordFailure sets next_attempt_at = NOW() + 1 min.
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("first Drain: %v", err)
	}

	// sent_at must still be NULL (delivery not yet confirmed).
	var sentAt pgtype.Timestamptz
	if err := testPool.QueryRow(ctx,
		"SELECT sent_at FROM notifications WHERE organization_id = $1 AND user_id = $2 AND channel = 'email'",
		org.ID, user.ID).Scan(&sentAt); err != nil {
		t.Fatalf("select sent_at after first fail: %v", err)
	}
	if sentAt.Valid {
		t.Error("sent_at is NOT NULL after failed delivery, want NULL")
	}

	// failed_permanently must be false (only 1 attempt so far, max = 3).
	var perm bool
	if err := testPool.QueryRow(ctx,
		"SELECT failed_permanently FROM notifications WHERE organization_id = $1 AND user_id = $2 AND channel = 'email'",
		org.ID, user.ID).Scan(&perm); err != nil {
		t.Fatalf("select failed_permanently: %v", err)
	}
	if perm {
		t.Error("failed_permanently is TRUE after 1 attempt, want FALSE")
	}

	// Advance the lease_expires_at so the row is immediately claimable again.
	if _, err := testPool.Exec(ctx,
		"UPDATE notifications SET lease_expires_at = NOW() - INTERVAL '1 second' WHERE organization_id = $1 AND user_id = $2 AND channel = 'email'",
		org.ID, user.ID,
	); err != nil {
		t.Fatalf("advance lease: %v", err)
	}

	// Second drain: delivery succeeds (failOnce only fails once).
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("second Drain: %v", err)
	}
	var sentAt2 pgtype.Timestamptz
	if err := testPool.QueryRow(ctx,
		"SELECT sent_at FROM notifications WHERE organization_id = $1 AND user_id = $2 AND channel = 'email'",
		org.ID, user.ID).Scan(&sentAt2); err != nil {
		t.Fatalf("select sent_at after second attempt: %v", err)
	}
	if !sentAt2.Valid {
		t.Error("sent_at is NULL after successful second attempt, want non-NULL")
	}
}

// TestEmailWorker_PermanentFailure verifies that after max_attempts the row
// is marked failed_permanently and the claim query no longer picks it up.
func TestEmailWorker_PermanentFailure(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	// Always-failing provider.
	alwaysFail := &failingProvider{failN: 99, failCount: new(int)}
	cfg := testCfg()
	sender := email.NewSenderWithProvider(alwaysFail, email.SenderConfig{
		FromAddress: cfg.EmailFromAddress,
		FromName:    cfg.EmailFromName,
		AppBaseURL:  cfg.AppBaseURL,
	}, discardLog())

	_ = seedEmailNotification(t, ctx, testPool, org.ID, user.ID)

	worker := notifworker.NewEmailWorker(testPool, sender, cfg.AppBaseURL, time.Minute, discardLog())

	// Run 3 drain cycles (each one claims and fails; advance lease between runs).
	for i := 0; i < 3; i++ {
		if err := worker.Drain(ctx); err != nil {
			t.Fatalf("Drain cycle %d: %v", i+1, err)
		}
		// Advance lease so the next cycle can claim the row.
		if i < 2 {
			if _, err := testPool.Exec(ctx,
				"UPDATE notifications SET lease_expires_at = NOW() - INTERVAL '1 second' WHERE organization_id = $1 AND user_id = $2 AND channel = 'email'",
				org.ID, user.ID,
			); err != nil {
				t.Fatalf("advance lease (cycle %d): %v", i+1, err)
			}
		}
	}

	// After 3 failures, failed_permanently must be TRUE.
	var perm bool
	var attemptCount int32
	if err := testPool.QueryRow(ctx,
		"SELECT failed_permanently, attempt_count FROM notifications WHERE organization_id = $1 AND user_id = $2 AND channel = 'email'",
		org.ID, user.ID).Scan(&perm, &attemptCount); err != nil {
		t.Fatalf("select state after 3 failures: %v", err)
	}
	if !perm {
		t.Errorf("failed_permanently = FALSE after %d attempts, want TRUE", attemptCount)
	}

	// A 4th drain must not claim the row (claim query filters attempt_count < max_attempts).
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("4th Drain: %v", err)
	}
	var attemptCount2 int32
	if err := testPool.QueryRow(ctx,
		"SELECT attempt_count FROM notifications WHERE organization_id = $1 AND user_id = $2 AND channel = 'email'",
		org.ID, user.ID).Scan(&attemptCount2); err != nil {
		t.Fatalf("select attempt_count after 4th drain: %v", err)
	}
	if attemptCount2 != 3 {
		t.Errorf("attempt_count = %d after permanently-failed + 4th drain, want 3", attemptCount2)
	}
}

// TestEmailWorker_SkipInApp verifies that the worker does not claim in_app
// notification rows. We seed a manual in_app row (sent_at = NOW()) alongside
// a pending email row and confirm only the email row is affected by the worker.
func TestEmailWorker_SkipInApp(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	mailer := &email.NoOpProvider{}
	cfg := testCfg()
	sender := email.NewSenderWithProvider(mailer, email.SenderConfig{
		FromAddress: cfg.EmailFromAddress,
		FromName:    cfg.EmailFromName,
		AppBaseURL:  cfg.AppBaseURL,
	}, discardLog())

	// Seed an outbox entry and insert both in_app (sent_at = NOW()) and
	// email (sent_at = NULL) notification rows manually.
	var outboxID pgtype.UUID
	if err := testPool.QueryRow(ctx, `
		INSERT INTO notification_outbox (organization_id, event_type, entity_type, entity_id, payload)
		VALUES ($1, 'registration_approved', 'tournament_registrations', $1, '{}')
		RETURNING id
	`, org.ID).Scan(&outboxID); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	// in_app row — already delivered (sent_at = NOW()).
	if _, err := testPool.Exec(ctx, `
		INSERT INTO notifications (organization_id, user_id, outbox_id, channel, event_type,
		                           entity_type, entity_id, payload, sent_at)
		VALUES ($1, $2, $3, 'in_app', 'registration_approved',
		        'tournament_registrations', $1, '{}', NOW())
	`, org.ID, user.ID, outboxID); err != nil {
		t.Fatalf("insert in_app row: %v", err)
	}
	// email row — pending (sent_at = NULL).
	if _, err := testPool.Exec(ctx, `
		INSERT INTO notifications (organization_id, user_id, outbox_id, channel, event_type,
		                           entity_type, entity_id, payload)
		VALUES ($1, $2, $3, 'email', 'registration_approved',
		        'tournament_registrations', $1, '{}')
	`, org.ID, user.ID, outboxID); err != nil {
		t.Fatalf("insert email row: %v", err)
	}

	worker := notifworker.NewEmailWorker(testPool, sender, cfg.AppBaseURL, time.Minute, discardLog())
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	// email row must now have sent_at set.
	var emailSentAt pgtype.Timestamptz
	if err := testPool.QueryRow(ctx,
		"SELECT sent_at FROM notifications WHERE organization_id = $1 AND user_id = $2 AND channel = 'email'",
		org.ID, user.ID).Scan(&emailSentAt); err != nil {
		t.Fatalf("select email sent_at after drain: %v", err)
	}
	if !emailSentAt.Valid {
		t.Error("email sent_at is NULL after worker drain, want non-NULL")
	}

	// Worker must have delivered exactly 1 email (not 2 — in_app must be skipped).
	if mailer.Count() != 1 {
		t.Errorf("mailer.Count() = %d, want 1 (in_app must not trigger email send)", mailer.Count())
	}
}

// ── failingProvider ────────────────────────────────────────────────────────────

// failingProvider is a test email.Provider that fails the first failN send
// calls then succeeds.
type failingProvider struct {
	failN     int
	failCount *int
}

func (f *failingProvider) Send(_ context.Context, _ email.Message) error {
	if *f.failCount < f.failN {
		*f.failCount++
		return fmt.Errorf("failingProvider: simulated delivery failure %d", *f.failCount)
	}
	return nil
}
