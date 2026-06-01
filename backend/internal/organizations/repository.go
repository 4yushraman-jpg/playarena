package organizations

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Repository provides data access for the organizations domain.
type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(queries *db.Queries, pool *pgxpool.Pool) *Repository {
	return &Repository{queries: queries, pool: pool}
}

// ── reads ─────────────────────────────────────────────────────────────────────

func (r *Repository) GetByID(ctx context.Context, id pgtype.UUID) (*db.Organization, error) {
	org, err := r.queries.GetOrganizationByID(ctx, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrOrganizationNotFound
		}
		return nil, err
	}
	return &org, nil
}

func (r *Repository) GetBySlug(ctx context.Context, slug string) (*db.Organization, error) {
	org, err := r.queries.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrOrganizationNotFound
		}
		return nil, err
	}
	return &org, nil
}

// List returns a bounded page of organizations. Limit and offset are pre-validated
// by the service layer.
func (r *Repository) List(ctx context.Context, params ListParams) ([]db.Organization, error) {
	return r.queries.ListOrganizationsPaginated(ctx, db.ListOrganizationsPaginatedParams{
		PageLimit:  params.Limit,
		PageOffset: params.Offset,
	})
}

// ── writes (transactional) ────────────────────────────────────────────────────

// createOrgTxParams bundles everything needed by CreateWithOwnerGrant.
// newData is intentionally absent — the repository computes it from the actual
// DB-returned org after the INSERT (M3 fix: audit log contains complete data).
type createOrgTxParams struct {
	orgParams db.CreateOrganizationParams
	creatorID pgtype.UUID
}

// CreateWithOwnerGrant atomically:
//  1. Inserts the new organization.
//  2. Looks up the org_owner system role.
//  3. Grants the creator the org_owner role in the new org.
//  4. Appends an audit log record using the actual DB return values (M3 fix).
func (r *Repository) CreateWithOwnerGrant(ctx context.Context, p createOrgTxParams) (*db.Organization, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	org, err := qtx.CreateOrganization(ctx, p.orgParams)
	if err != nil {
		if pgutil.IsUniqueViolation(err, "uq_organizations_slug") {
			return nil, ErrSlugAlreadyTaken
		}
		return nil, err
	}

	role, err := qtx.GetRoleBySlug(ctx, "org_owner")
	if err != nil {
		return nil, fmt.Errorf("org_owner role not found: %w", err)
	}

	if err := qtx.GrantRoleToUserInOrg(ctx, db.GrantRoleToUserInOrgParams{
		UserID:         p.creatorID,
		OrganizationID: org.ID,
		RoleID:         role.ID,
		GrantedBy:      p.creatorID,
	}); err != nil {
		return nil, err
	}

	// M3 fix: compute newData from the actual DB-returned org row, which includes
	// all generated fields (id, status, created_at, updated_at, settings).
	newData, err := orgToAuditJSON(&org)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: org.ID,
		UserID:         p.creatorID,
		Action:         db.AuditActionCreate,
		EntityType:     "organizations",
		EntityID:       org.ID,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &org, nil
}

// updateOrgTxParams bundles everything needed by UpdateWithAudit.
// newData is intentionally absent — computed from DB return value (M4 fix).
type updateOrgTxParams struct {
	updateParams db.UpdateOrganizationParams
	actorID      pgtype.UUID
	oldData      []byte // JSON snapshot captured before the update
}

// UpdateWithAudit atomically updates the organization and appends an audit record.
// The audit new_data is derived from the actual updated row returned by the DB,
// not from a pre-computed projection (M4 fix).
func (r *Repository) UpdateWithAudit(ctx context.Context, p updateOrgTxParams) (*db.Organization, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	org, err := qtx.UpdateOrganization(ctx, p.updateParams)
	if err != nil {
		return nil, err
	}

	// M4 fix: use the actual updated row for new_data instead of a projection.
	newData, err := orgToAuditJSON(&org)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: org.ID,
		UserID:         p.actorID,
		Action:         db.AuditActionUpdate,
		EntityType:     "organizations",
		EntityID:       org.ID,
		OldData:        p.oldData,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &org, nil
}

// deleteOrgTxParams bundles everything needed by DeleteWithAudit.
type deleteOrgTxParams struct {
	orgID   pgtype.UUID
	actorID pgtype.UUID
	oldData []byte // JSON snapshot of the org being deleted
}

// DeleteWithAudit atomically deletes the organization and appends an audit record.
func (r *Repository) DeleteWithAudit(ctx context.Context, p deleteOrgTxParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	if err := qtx.DeleteOrganization(ctx, p.orgID); err != nil {
		return err
	}

	// organization_id left NULL: the org is deleted, FK is ON DELETE SET NULL.
	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		UserID:     p.actorID,
		Action:     db.AuditActionDelete,
		EntityType: "organizations",
		EntityID:   p.orgID,
		OldData:    p.oldData,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// orgToAuditJSON marshals a complete org snapshot suitable for audit_logs.
// This is called AFTER the DB operation returns, so all generated fields
// (id, status, created_at, updated_at, logo_url, settings) are included.
func orgToAuditJSON(o *db.Organization) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":          pgutil.UUIDToString(o.ID),
		"name":        o.Name,
		"slug":        o.Slug,
		"type":        string(o.Type),
		"status":      string(o.Status),
		"description": o.Description,
		"website":     o.Website,
		"email":       o.Email,
		"phone":       o.Phone,
		"country":     o.Country,
		"city":        o.City,
		"created_at":  o.CreatedAt.Time.UTC().Format(time.RFC3339),
		"updated_at":  o.UpdatedAt.Time.UTC().Format(time.RFC3339),
	})
}
