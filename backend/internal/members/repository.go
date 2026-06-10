package members

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

// Repository provides data access for the members domain.
type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(queries *db.Queries, pool *pgxpool.Pool) *Repository {
	return &Repository{queries: queries, pool: pool}
}

// ── reads ─────────────────────────────────────────────────────────────────────

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

func (r *Repository) GetUserByID(ctx context.Context, id pgtype.UUID) (*db.User, error) {
	u, err := r.queries.GetUserByID(ctx, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *Repository) GetRoleBySlug(ctx context.Context, slug string) (*db.Role, error) {
	role, err := r.queries.GetRoleBySlug(ctx, slug)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}
	return &role, nil
}

func (r *Repository) ListOrgMembersWithRoles(ctx context.Context, orgID pgtype.UUID) ([]db.ListOrgMembersWithRolesRow, error) {
	return r.queries.ListOrgMembersWithRoles(ctx, orgID)
}

func (r *Repository) GetUserGrantsInOrg(ctx context.Context, userID, orgID pgtype.UUID) ([]db.GetUserGrantsInOrgRow, error) {
	return r.queries.GetUserGrantsInOrg(ctx, db.GetUserGrantsInOrgParams{
		UserID: userID,
		OrgID:  orgID,
	})
}

func (r *Repository) CountActiveOrgOwners(ctx context.Context, orgID pgtype.UUID) (int64, error) {
	return r.queries.CountActiveOrgOwnersByOrg(ctx, orgID)
}

// ── writes ────────────────────────────────────────────────────────────────────

type grantRoleTxParams struct {
	grantParams db.GrantOrgRoleParams
	actorID     pgtype.UUID
}

// GrantRoleWithAudit atomically inserts a role grant and writes an audit record.
func (r *Repository) GrantRoleWithAudit(ctx context.Context, p grantRoleTxParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	if err := qtx.GrantOrgRole(ctx, p.grantParams); err != nil {
		return err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: p.grantParams.OrganizationID,
		UserID:         p.actorID,
		Action:         db.AuditActionPermissionChange,
		EntityType:     "users",
		EntityID:       p.grantParams.UserID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

type revokeRoleTxParams struct {
	revokeParams db.RevokeRoleFromUserInOrgParams
	orgID        pgtype.UUID
	actorID      pgtype.UUID
}

// RevokeRoleWithAudit atomically deletes a role grant and writes an audit record.
// Returns ErrGrantNotFound when the grant does not exist.
func (r *Repository) RevokeRoleWithAudit(ctx context.Context, p revokeRoleTxParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	n, err := qtx.RevokeRoleFromUserInOrg(ctx, p.revokeParams)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrGrantNotFound
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: p.orgID,
		UserID:         p.actorID,
		Action:         db.AuditActionPermissionChange,
		EntityType:     "users",
		EntityID:       p.revokeParams.UserID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
