package webhookworker

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

// Repository provides the data-access layer for the WebhookWorker.
type Repository struct {
	queries *db.Queries
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{queries: db.New(pool)}
}

// ClaimBatch claims up to batchSize pending webhook deliveries for delivery.
func (r *Repository) ClaimBatch(ctx context.Context, maxAttempts, batchSize int32) ([]db.WebhookDelivery, error) {
	return r.queries.ClaimWebhookDeliveriesForDelivery(ctx, db.ClaimWebhookDeliveriesForDeliveryParams{
		MaxAttempts: maxAttempts,
		BatchSize:   batchSize,
	})
}

// GetEndpoint fetches the webhook endpoint for the given delivery row.
func (r *Repository) GetEndpoint(ctx context.Context, deliveryID pgtype.UUID) (*db.WebhookEndpoint, error) {
	ep, err := r.queries.GetWebhookEndpointForDelivery(ctx, deliveryID)
	if err != nil {
		return nil, err
	}
	return &ep, nil
}

// RecordSuccess marks a delivery as successfully sent.
func (r *Repository) RecordSuccess(ctx context.Context, id pgtype.UUID) error {
	return r.queries.RecordWebhookDeliverySuccess(ctx, id)
}

// RecordFailure records a failed delivery attempt.
func (r *Repository) RecordFailure(ctx context.Context, id pgtype.UUID, failedPermanently bool, nextAttemptAt pgtype.Timestamptz) error {
	return r.queries.RecordWebhookDeliveryFailure(ctx, db.RecordWebhookDeliveryFailureParams{
		ID:                id,
		FailedPermanently: failedPermanently,
		NextAttemptAt:     nextAttemptAt,
	})
}
