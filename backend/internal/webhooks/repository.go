package webhooks

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

// Repository provides data access for the webhooks domain.
type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(queries *db.Queries, pool *pgxpool.Pool) *Repository {
	return &Repository{queries: queries, pool: pool}
}

// GetOrgBySlug resolves an organization by its URL slug.
func (r *Repository) GetOrgBySlug(ctx context.Context, slug string) (*db.Organization, error) {
	org, err := r.queries.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrOrganizationNotFound
		}
		return nil, err
	}
	return &org, nil
}

// Create inserts a new webhook endpoint row.
func (r *Repository) Create(ctx context.Context, params db.CreateWebhookEndpointParams) (*db.WebhookEndpoint, error) {
	ep, err := r.queries.CreateWebhookEndpoint(ctx, params)
	if err != nil {
		return nil, err
	}
	return &ep, nil
}

// GetByID fetches a webhook endpoint scoped to the organization.
// Returns ErrWebhookNotFound when the row does not exist.
func (r *Repository) GetByID(ctx context.Context, id, orgID pgtype.UUID) (*db.WebhookEndpoint, error) {
	ep, err := r.queries.GetWebhookEndpointByID(ctx, db.GetWebhookEndpointByIDParams{
		ID:             id,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrWebhookNotFound
		}
		return nil, err
	}
	return &ep, nil
}

// List returns all webhook endpoints for the organization (newest-first).
func (r *Repository) List(ctx context.Context, orgID pgtype.UUID) ([]db.WebhookEndpoint, error) {
	return r.queries.ListWebhookEndpoints(ctx, orgID)
}

// UpdateActive toggles the active flag on a webhook endpoint.
// Returns ErrWebhookNotFound when the row does not exist.
func (r *Repository) UpdateActive(ctx context.Context, id, orgID pgtype.UUID, active bool) (*db.WebhookEndpoint, error) {
	ep, err := r.queries.UpdateWebhookEndpointActive(ctx, db.UpdateWebhookEndpointActiveParams{
		ID:             id,
		OrganizationID: orgID,
		Active:         active,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrWebhookNotFound
		}
		return nil, err
	}
	return &ep, nil
}

// Delete removes a webhook endpoint. Returns ErrWebhookNotFound when not found.
func (r *Repository) Delete(ctx context.Context, id, orgID pgtype.UUID) error {
	n, err := r.queries.DeleteWebhookEndpoint(ctx, db.DeleteWebhookEndpointParams{
		ID:             id,
		OrganizationID: orgID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrWebhookNotFound
	}
	return nil
}
