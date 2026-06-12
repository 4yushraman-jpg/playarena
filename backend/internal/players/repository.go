package players

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Repository provides data access for the players domain.
type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(queries *db.Queries, pool *pgxpool.Pool) *Repository {
	return &Repository{queries: queries, pool: pool}
}

// ── reads ─────────────────────────────────────────────────────────────────────

// GetOrgBySlug resolves an organization by its URL slug. Used to enforce
// multi-tenant context before any player operation.
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

// GetByID fetches a single player by UUID within an organization.
// No status filter is applied deliberately: inactive (soft-deleted) players
// must remain retrievable so that foreign-key references in team_memberships
// and match_events continue to resolve. The caller receives the full record
// including the current status and can decide how to present it.
func (r *Repository) GetByID(ctx context.Context, id, orgID pgtype.UUID) (*db.Player, error) {
	p, err := r.queries.GetPlayerByID(ctx, db.GetPlayerByIDParams{
		ID:             id,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrPlayerNotFound
		}
		return nil, err
	}
	return &p, nil
}

// List returns a paginated page of non-inactive players for an org.
func (r *Repository) List(ctx context.Context, orgID pgtype.UUID, params ListParams) ([]db.Player, error) {
	return r.queries.ListPlayersPaginated(ctx, db.ListPlayersPaginatedParams{
		OrganizationID: orgID,
		StatusFilter:   params.StatusFilter,
		SearchQuery:    params.Search,
		PageLimit:      params.Limit,
		PageOffset:     params.Offset,
	})
}

// Count returns the total number of non-inactive players matching the filters.
func (r *Repository) Count(ctx context.Context, orgID pgtype.UUID, params ListParams) (int64, error) {
	return r.queries.CountPlayersByOrganization(ctx, db.CountPlayersByOrganizationParams{
		OrganizationID: orgID,
		StatusFilter:   params.StatusFilter,
		SearchQuery:    params.Search,
	})
}

// GetPrimaryMediaURL returns the CDN URL of the most recent primary
// media_attachment for a player. Returns nil, nil when no attachment exists.
// Prefers is_primary = true; falls back to most-recently-uploaded.
func (r *Repository) GetPrimaryMediaURL(ctx context.Context, playerID, orgID pgtype.UUID) (*string, error) {
	var fileURL string
	err := r.pool.QueryRow(ctx, `
		SELECT file_url
		FROM media_attachments
		WHERE entity_type = 'player'
		  AND entity_id = $1
		  AND organization_id = $2
		ORDER BY is_primary DESC, created_at DESC
		LIMIT 1
	`, playerID, orgID).Scan(&fileURL)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &fileURL, nil
}

// ── GP-1: global PlayerProfile (user-owned) ───────────────────────────────────

// GetProfileByUserID returns the caller's canonical (non-archived) profile.
// Returns ErrPlayerNotFound when the user has no profile.
func (r *Repository) GetProfileByUserID(ctx context.Context, userID pgtype.UUID) (*db.Player, error) {
	p, err := r.queries.GetPlayerProfileByUserID(ctx, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrPlayerNotFound
		}
		return nil, err
	}
	return &p, nil
}

// GetProfileByID returns a single non-archived profile by id, regardless of org.
// Visibility filtering is the caller's responsibility.
func (r *Repository) GetProfileByID(ctx context.Context, id pgtype.UUID) (*db.Player, error) {
	p, err := r.queries.GetPlayerProfileByID(ctx, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrPlayerNotFound
		}
		return nil, err
	}
	return &p, nil
}

// CreateGlobalProfile inserts an owner-created, org-less PlayerProfile.
// A duplicate (the user already has a canonical profile) maps to ErrProfileExists.
func (r *Repository) CreateGlobalProfile(ctx context.Context, params db.CreateGlobalPlayerProfileParams) (*db.Player, error) {
	p, err := r.queries.CreateGlobalPlayerProfile(ctx, params)
	if err != nil {
		if pgutil.IsUniqueViolation(err, "uq_players_user_id") {
			return nil, ErrProfileExists
		}
		return nil, err
	}
	return &p, nil
}

// UpdateOwnProfile applies an identity-field update bound to the owner.
// Returns ErrPlayerNotFound when no canonical profile exists for the user.
func (r *Repository) UpdateOwnProfile(ctx context.Context, params db.UpdateOwnPlayerProfileParams) (*db.Player, error) {
	p, err := r.queries.UpdateOwnPlayerProfile(ctx, params)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrPlayerNotFound
		}
		return nil, err
	}
	return &p, nil
}

// ── transactional writes ──────────────────────────────────────────────────────

type createPlayerTxParams struct {
	createParams db.CreatePlayerParams
	actorID      pgtype.UUID
}

// CreateWithAudit atomically inserts the player and writes an audit log record.
// newData is derived from the actual DB-returned row (not from the request).
func (r *Repository) CreateWithAudit(ctx context.Context, p createPlayerTxParams) (*db.Player, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	player, err := qtx.CreatePlayer(ctx, p.createParams)
	if err != nil {
		return nil, err
	}

	newData, err := playerToAuditJSON(&player)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: player.OrganizationID,
		UserID:         p.actorID,
		Action:         db.AuditActionCreate,
		EntityType:     "players",
		EntityID:       player.ID,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &player, nil
}

type updatePlayerTxParams struct {
	updateParams db.UpdatePlayerParams
	actorID      pgtype.UUID
	oldData      []byte // snapshot captured before the update
}

// UpdateWithAudit atomically updates the player and writes an audit log record.
// newData is derived from the actual updated row returned by the DB.
func (r *Repository) UpdateWithAudit(ctx context.Context, p updatePlayerTxParams) (*db.Player, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	player, err := qtx.UpdatePlayer(ctx, p.updateParams)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrPlayerNotFound
		}
		return nil, err
	}

	newData, err := playerToAuditJSON(&player)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: player.OrganizationID,
		UserID:         p.actorID,
		Action:         db.AuditActionUpdate,
		EntityType:     "players",
		EntityID:       player.ID,
		OldData:        p.oldData,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &player, nil
}

type softDeletePlayerTxParams struct {
	id      pgtype.UUID
	orgID   pgtype.UUID
	actorID pgtype.UUID
	oldData []byte // snapshot captured before the soft delete
}

// SoftDeleteWithAudit atomically sets the player status to inactive and writes
// an audit log record with action=delete. Records are never hard-deleted.
func (r *Repository) SoftDeleteWithAudit(ctx context.Context, p softDeletePlayerTxParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	_, err = qtx.SoftDeletePlayer(ctx, db.SoftDeletePlayerParams{
		ID:             p.id,
		OrganizationID: p.orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrPlayerNotFound
		}
		return err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: p.orgID,
		UserID:         p.actorID,
		Action:         db.AuditActionDelete,
		EntityType:     "players",
		EntityID:       p.id,
		OldData:        p.oldData,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// playerToAuditJSON marshals a complete player snapshot for audit_logs.
// Called AFTER the DB operation so all generated fields are present.
func playerToAuditJSON(p *db.Player) ([]byte, error) {
	var dob *string
	if p.DateOfBirth.Valid {
		s := p.DateOfBirth.Time.Format("2006-01-02")
		dob = &s
	}
	var userID *string
	if p.UserID.Valid {
		uid := pgutil.UUIDToString(p.UserID)
		userID = &uid
	}
	return json.Marshal(map[string]any{
		"id":              pgutil.UUIDToString(p.ID),
		"organization_id": pgutil.UUIDToString(p.OrganizationID),
		"user_id":         userID,
		"display_name":    p.DisplayName,
		"jersey_number":   p.JerseyNumber,
		"position":        p.Position,
		"height_cm":       p.HeightCm,
		"weight_kg":       p.WeightKg,
		"dominant_hand":   p.DominantHand,
		"nationality":     p.Nationality,
		"date_of_birth":   dob,
		"status":          string(p.Status),
		"bio":             p.Bio,
		"created_at":      p.CreatedAt.Time.UTC().Format(time.RFC3339),
		"updated_at":      p.UpdatedAt.Time.UTC().Format(time.RFC3339),
	})
}
