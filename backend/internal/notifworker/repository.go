package notifworker

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

// Repository provides the data-access layer for the EmailWorker.
// All methods are safe for concurrent use.
type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(queries *db.Queries, pool *pgxpool.Pool) *Repository {
	return &Repository{queries: queries, pool: pool}
}

// ClaimBatch claims up to batchSize pending email notifications for delivery.
// Rows are claimed using FOR UPDATE SKIP LOCKED so concurrent worker instances
// do not double-deliver. attempt_count is incremented atomically at claim time.
func (r *Repository) ClaimBatch(ctx context.Context, maxAttempts, batchSize int32) ([]db.Notification, error) {
	return r.queries.ClaimEmailNotificationsForDelivery(ctx, db.ClaimEmailNotificationsForDeliveryParams{
		MaxAttempts: maxAttempts,
		BatchSize:   batchSize,
	})
}

// RecordSuccess marks an email notification as successfully delivered.
func (r *Repository) RecordSuccess(ctx context.Context, id pgtype.UUID) error {
	return r.queries.RecordEmailDeliverySuccess(ctx, id)
}

// RecordFailure records a failed delivery attempt.
// failedPermanently should be true when attempt_count >= max_attempts.
// nextAttemptAt is the earliest time the row may be claimed again.
func (r *Repository) RecordFailure(ctx context.Context, id pgtype.UUID, failedPermanently bool, nextAttemptAt pgtype.Timestamptz) error {
	return r.queries.RecordEmailDeliveryFailure(ctx, db.RecordEmailDeliveryFailureParams{
		ID:                id,
		FailedPermanently: failedPermanently,
		NextAttemptAt:     nextAttemptAt,
	})
}

// GetUserByID fetches a user by ID. Used to look up the recipient email address
// and display name before email delivery.
func (r *Repository) GetUserByID(ctx context.Context, id pgtype.UUID) (*db.User, error) {
	user, err := r.queries.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetOrgSlugByID fetches the URL slug for an organization. Used to construct
// the notifications deep-link in the email body.
func (r *Repository) GetOrgSlugByID(ctx context.Context, id pgtype.UUID) (string, error) {
	org, err := r.queries.GetOrganizationByID(ctx, id)
	if err != nil {
		return "", err
	}
	return org.Slug, nil
}

// CountDeadLetters returns the number of email notification rows with
// failed_permanently = TRUE. Used by the background metrics scraper.
func (r *Repository) CountDeadLetters(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE channel = 'email' AND failed_permanently = TRUE`,
	).Scan(&n)
	return n, err
}
