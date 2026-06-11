package teams

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

// Repository provides data access for the teams domain.
type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(queries *db.Queries, pool *pgxpool.Pool) *Repository {
	return &Repository{queries: queries, pool: pool}
}

// ── reads ─────────────────────────────────────────────────────────────────────

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

// GetTeamByID fetches a single team by UUID within an organization.
// No status filter: disbanded (soft-deleted) teams are intentionally returned
// so that historical match and ranking references remain resolvable.
func (r *Repository) GetTeamByID(ctx context.Context, id, orgID pgtype.UUID) (*db.Team, error) {
	t, err := r.queries.GetTeamByID(ctx, db.GetTeamByIDParams{
		ID:             id,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrTeamNotFound
		}
		return nil, err
	}
	return &t, nil
}

// GetTeamBySlug fetches a single team by slug within an organization.
func (r *Repository) GetTeamBySlug(ctx context.Context, slug string, orgID pgtype.UUID) (*db.Team, error) {
	t, err := r.queries.GetTeamBySlug(ctx, db.GetTeamBySlugParams{
		Slug:           slug,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrTeamNotFound
		}
		return nil, err
	}
	return &t, nil
}

// GetPlayerByID verifies a player exists within an organization and returns
// their display_name. Used during membership creation to confirm the player
// belongs to the correct org and to embed the name in the membership response.
func (r *Repository) GetPlayerByID(ctx context.Context, id, orgID pgtype.UUID) (string, error) {
	p, err := r.queries.GetPlayerByID(ctx, db.GetPlayerByIDParams{
		ID:             id,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", ErrPlayerNotFound
		}
		return "", err
	}
	return p.DisplayName, nil
}

// List returns a paginated page of non-disbanded teams for an org.
func (r *Repository) List(ctx context.Context, orgID pgtype.UUID, params ListParams) ([]db.Team, error) {
	return r.queries.ListTeamsPaginated(ctx, db.ListTeamsPaginatedParams{
		OrganizationID: orgID,
		StatusFilter:   params.StatusFilter,
		SearchQuery:    params.Search,
		PageLimit:      params.Limit,
		PageOffset:     params.Offset,
	})
}

// Count returns the total count of non-disbanded teams matching the filters.
func (r *Repository) Count(ctx context.Context, orgID pgtype.UUID, params ListParams) (int64, error) {
	return r.queries.CountTeamsByOrganization(ctx, db.CountTeamsByOrganizationParams{
		OrganizationID: orgID,
		StatusFilter:   params.StatusFilter,
		SearchQuery:    params.Search,
	})
}

// GetActiveMembershipByPlayer checks whether a player already holds an active
// membership on ANY team within the organization.
// Enforces the rule: one active team membership per player per organization.
// Returns nil when no active membership exists (safe to create a new one).
func (r *Repository) GetActiveMembershipByPlayer(ctx context.Context, playerID, orgID pgtype.UUID) (*db.TeamMembership, error) {
	m, err := r.queries.GetActiveMembershipByPlayer(ctx, db.GetActiveMembershipByPlayerParams{
		PlayerID:       playerID,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// GetActiveMembership checks whether a player already has an active membership
// on the given team. Returns nil error when no such membership exists.
func (r *Repository) GetActiveMembership(ctx context.Context, teamID, playerID, orgID pgtype.UUID) (*db.TeamMembership, error) {
	m, err := r.queries.GetActiveMembershipByTeamAndPlayer(ctx, db.GetActiveMembershipByTeamAndPlayerParams{
		TeamID:         teamID,
		PlayerID:       playerID,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // no active membership — OK to create
		}
		return nil, err
	}
	return &m, nil
}

// ListActiveMembers returns all active memberships for a team.
func (r *Repository) ListActiveMembers(ctx context.Context, teamID, orgID pgtype.UUID) ([]db.TeamMembership, error) {
	return r.queries.ListActiveMembersByTeam(ctx, db.ListActiveMembersByTeamParams{
		TeamID:         teamID,
		OrganizationID: orgID,
	})
}

// memberWithName holds a team_membership row joined with the player's
// display_name so the roster API can return display-ready data in one query.
type memberWithName struct {
	ID                pgtype.UUID
	TeamID            pgtype.UUID
	PlayerID          pgtype.UUID
	OrganizationID    pgtype.UUID
	Role              string
	JerseyNumber      pgtype.Text
	Status            string
	JoinedAt          pgtype.Timestamptz
	LeftAt            pgtype.Timestamptz
	Notes             pgtype.Text
	CreatedAt         pgtype.Timestamptz
	UpdatedAt         pgtype.Timestamptz
	PlayerDisplayName string
}

// ListActiveMembersWithNames returns all active memberships for a team joined
// with each player's display_name, ordered alphabetically by player name.
// Uses a raw JOIN query to avoid N+1 lookups.
func (r *Repository) ListActiveMembersWithNames(ctx context.Context, teamID, orgID pgtype.UUID) ([]memberWithName, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			tm.id, tm.team_id, tm.player_id, tm.organization_id,
			tm.role, tm.jersey_number, tm.status,
			tm.joined_at, tm.left_at, tm.notes,
			tm.created_at, tm.updated_at,
			p.display_name
		FROM team_memberships tm
		JOIN players p ON p.id = tm.player_id
		WHERE tm.team_id = $1
		  AND tm.organization_id = $2
		  AND tm.status = 'active'
		ORDER BY p.display_name ASC
	`, teamID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []memberWithName
	for rows.Next() {
		var m memberWithName
		if err := rows.Scan(
			&m.ID, &m.TeamID, &m.PlayerID, &m.OrganizationID,
			&m.Role, &m.JerseyNumber, &m.Status,
			&m.JoinedAt, &m.LeftAt, &m.Notes,
			&m.CreatedAt, &m.UpdatedAt,
			&m.PlayerDisplayName,
		); err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// GetMembershipByID fetches a membership by UUID within an organization.
func (r *Repository) GetMembershipByID(ctx context.Context, id, orgID pgtype.UUID) (*db.TeamMembership, error) {
	m, err := r.queries.GetMembershipByID(ctx, db.GetMembershipByIDParams{
		ID:             id,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrMembershipNotFound
		}
		return nil, err
	}
	return &m, nil
}

// ── transactional writes ──────────────────────────────────────────────────────

type createTeamTxParams struct {
	createParams db.CreateTeamParams
	actorID      pgtype.UUID
}

// CreateWithAudit atomically inserts the team and writes a create audit record.
func (r *Repository) CreateWithAudit(ctx context.Context, p createTeamTxParams) (*db.Team, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	team, err := qtx.CreateTeam(ctx, p.createParams)
	if err != nil {
		if pgutil.IsUniqueViolation(err, "uq_teams_org_slug") {
			return nil, ErrSlugAlreadyTaken
		}
		return nil, err
	}

	newData, err := teamToAuditJSON(&team)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: team.OrganizationID,
		UserID:         p.actorID,
		Action:         db.AuditActionCreate,
		EntityType:     "teams",
		EntityID:       team.ID,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &team, nil
}

type updateTeamTxParams struct {
	updateParams db.UpdateTeamParams
	actorID      pgtype.UUID
	oldData      []byte
}

// UpdateWithAudit atomically updates the team and writes an update audit record.
func (r *Repository) UpdateWithAudit(ctx context.Context, p updateTeamTxParams) (*db.Team, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	team, err := qtx.UpdateTeam(ctx, p.updateParams)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrTeamNotFound
		}
		return nil, err
	}

	newData, err := teamToAuditJSON(&team)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: team.OrganizationID,
		UserID:         p.actorID,
		Action:         db.AuditActionUpdate,
		EntityType:     "teams",
		EntityID:       team.ID,
		OldData:        p.oldData,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &team, nil
}

type disbandTeamTxParams struct {
	id      pgtype.UUID
	orgID   pgtype.UUID
	actorID pgtype.UUID
	oldData []byte
}

// DisbandWithAudit atomically sets the team status to disbanded and writes a
// delete audit record. Records are never hard-deleted.
func (r *Repository) DisbandWithAudit(ctx context.Context, p disbandTeamTxParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	_, err = qtx.DisbandTeam(ctx, db.DisbandTeamParams{
		ID:             p.id,
		OrganizationID: p.orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrTeamNotFound
		}
		return err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: p.orgID,
		UserID:         p.actorID,
		Action:         db.AuditActionDelete,
		EntityType:     "teams",
		EntityID:       p.id,
		OldData:        p.oldData,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

type addMemberTxParams struct {
	createParams db.CreateMembershipParams
	actorID      pgtype.UUID
}

// AddMemberWithAudit atomically creates a team membership and writes a create
// audit record (entity_type = "team_memberships").
func (r *Repository) AddMemberWithAudit(ctx context.Context, p addMemberTxParams) (*db.TeamMembership, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	m, err := qtx.CreateMembership(ctx, p.createParams)
	if err != nil {
		return nil, err
	}

	newData, err := membershipToAuditJSON(&m)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: m.OrganizationID,
		UserID:         p.actorID,
		Action:         db.AuditActionCreate,
		EntityType:     "team_memberships",
		EntityID:       m.ID,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &m, nil
}

type removeMemberTxParams struct {
	id      pgtype.UUID
	teamID  pgtype.UUID
	orgID   pgtype.UUID
	actorID pgtype.UUID
	oldData []byte
}

// RemoveMemberWithAudit atomically soft-removes a team membership (sets
// status=released, left_at=NOW()) and writes a delete audit record.
func (r *Repository) RemoveMemberWithAudit(ctx context.Context, p removeMemberTxParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	_, err = qtx.RemoveMembership(ctx, db.RemoveMembershipParams{
		ID:             p.id,
		TeamID:         p.teamID,
		OrganizationID: p.orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrMembershipNotFound
		}
		return err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: p.orgID,
		UserID:         p.actorID,
		Action:         db.AuditActionDelete,
		EntityType:     "team_memberships",
		EntityID:       p.id,
		OldData:        p.oldData,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func teamToAuditJSON(t *db.Team) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":              pgutil.UUIDToString(t.ID),
		"organization_id": pgutil.UUIDToString(t.OrganizationID),
		"name":            t.Name,
		"short_name":      t.ShortName,
		"slug":            t.Slug,
		"description":     t.Description,
		"logo_url":        t.LogoUrl,
		"home_city":       t.HomeCity,
		"home_venue":      t.HomeVenue,
		"founded_year":    t.FoundedYear,
		"primary_color":   t.PrimaryColor,
		"secondary_color": t.SecondaryColor,
		"status":          string(t.Status),
		"created_at":      t.CreatedAt.Time.UTC().Format(time.RFC3339),
		"updated_at":      t.UpdatedAt.Time.UTC().Format(time.RFC3339),
	})
}

func membershipToAuditJSON(m *db.TeamMembership) ([]byte, error) {
	var leftAt *string
	if m.LeftAt.Valid {
		s := m.LeftAt.Time.UTC().Format(time.RFC3339)
		leftAt = &s
	}
	return json.Marshal(map[string]any{
		"id":              pgutil.UUIDToString(m.ID),
		"team_id":         pgutil.UUIDToString(m.TeamID),
		"player_id":       pgutil.UUIDToString(m.PlayerID),
		"organization_id": pgutil.UUIDToString(m.OrganizationID),
		"role":            string(m.Role),
		"jersey_number":   m.JerseyNumber,
		"status":          string(m.Status),
		"joined_at":       m.JoinedAt.Time.UTC().Format(time.RFC3339),
		"left_at":         leftAt,
		"notes":           m.Notes,
		"created_at":      m.CreatedAt.Time.UTC().Format(time.RFC3339),
		"updated_at":      m.UpdatedAt.Time.UTC().Format(time.RFC3339),
	})
}
