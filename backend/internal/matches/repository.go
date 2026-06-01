package matches

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

// Repository provides data access for the matches domain.
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

// GetTournamentByID fetches a tournament scoped to its org.
func (r *Repository) GetTournamentByID(ctx context.Context, id, orgID pgtype.UUID) (*db.Tournament, error) {
	t, err := r.queries.GetTournamentByID(ctx, db.GetTournamentByIDParams{
		ID:             id,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrTournamentNotFound
		}
		return nil, err
	}
	return &t, nil
}

// GetTeamByID fetches a team scoped to an org. Returns ErrTeamNotFound when the
// team does not exist or belongs to a different org (BOLA-safe cross-org guard).
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

// GetPlayerByID fetches a player scoped to an org. Returns ErrPlayerNotFound
// when the player does not exist or belongs to a different org.
func (r *Repository) GetPlayerByID(ctx context.Context, id, orgID pgtype.UUID) (*db.Player, error) {
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

// HasApprovedRegistrationByTeam reports whether a team holds an approved
// registration for the given tournament.
func (r *Repository) HasApprovedRegistrationByTeam(ctx context.Context, tournamentID, teamID pgtype.UUID) (bool, error) {
	_, err := r.queries.GetApprovedRegistrationByTeam(ctx, db.GetApprovedRegistrationByTeamParams{
		TournamentID: tournamentID,
		TeamID:       teamID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// HasApprovedRegistrationByPlayer reports whether a player holds an approved
// registration for the given tournament.
func (r *Repository) HasApprovedRegistrationByPlayer(ctx context.Context, tournamentID, playerID pgtype.UUID) (bool, error) {
	_, err := r.queries.GetApprovedRegistrationByPlayer(ctx, db.GetApprovedRegistrationByPlayerParams{
		TournamentID: tournamentID,
		PlayerID:     playerID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetByID fetches a single match by UUID scoped to an organization.
// Cancelled and completed matches are intentionally returned so that historical
// references (match_events, audit_logs) remain resolvable.
func (r *Repository) GetByID(ctx context.Context, id, orgID pgtype.UUID) (*db.Match, error) {
	m, err := r.queries.GetMatchByID(ctx, db.GetMatchByIDParams{
		ID:             id,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrMatchNotFound
		}
		return nil, err
	}
	return &m, nil
}

// List returns a paginated page of matches for an org.
func (r *Repository) List(ctx context.Context, orgID pgtype.UUID, params ListParams) ([]db.Match, error) {
	tidFilter := pgutil.ParseOptionalUUID(derefStr(params.TournamentFilter))
	return r.queries.ListMatchesPaginated(ctx, db.ListMatchesPaginatedParams{
		OrganizationID:     orgID,
		TournamentIDFilter: tidFilter,
		StatusFilter:       params.StatusFilter,
		SearchQuery:        params.Search,
		PageLimit:          params.Limit,
		PageOffset:         params.Offset,
	})
}

// Count returns the total count matching the same filters as List.
func (r *Repository) Count(ctx context.Context, orgID pgtype.UUID, params ListParams) (int64, error) {
	tidFilter := pgutil.ParseOptionalUUID(derefStr(params.TournamentFilter))
	return r.queries.CountMatches(ctx, db.CountMatchesParams{
		OrganizationID:     orgID,
		TournamentIDFilter: tidFilter,
		StatusFilter:       params.StatusFilter,
		SearchQuery:        params.Search,
	})
}

// ── transactional writes ──────────────────────────────────────────────────────

type createMatchTxParams struct {
	createParams   db.CreateMatchParams
	actorID        pgtype.UUID
	tournamentID   pgtype.UUID
	organizationID pgtype.UUID
}

// CreateWithAudit atomically:
//  1. Acquires a FOR SHARE lock on the tournament row — prevents a concurrent
//     tournament cancellation from racing with this insert.
//  2. Re-validates tournament.status == ongoing inside the transaction.
//  3. Inserts the match row.
//  4. Inserts a create audit record (new_data from the DB-returned row).
func (r *Repository) CreateWithAudit(ctx context.Context, p createMatchTxParams) (*db.Match, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	// Lock the tournament row to prevent a concurrent cancellation from
	// committing between this check and the match INSERT.
	status, err := qtx.LockTournamentForShare(ctx, db.LockTournamentForShareParams{
		ID:             p.tournamentID,
		OrganizationID: p.organizationID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrTournamentNotFound
		}
		return nil, err
	}
	if status != db.TournamentStatusOngoing {
		return nil, ErrTournamentNotOngoing
	}

	m, err := qtx.CreateMatch(ctx, p.createParams)
	if err != nil {
		return nil, err
	}

	newData, err := matchToAuditJSON(&m)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: m.OrganizationID,
		UserID:         p.actorID,
		Action:         db.AuditActionCreate,
		EntityType:     "matches",
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

type updateMatchTxParams struct {
	updateParams   db.UpdateMatchParams
	actorID        pgtype.UUID
	oldData        []byte
	lockTournament bool // true when transitioning to live/completed/abandoned
	tournamentID   pgtype.UUID
	organizationID pgtype.UUID
}

// UpdateWithAudit atomically updates a match and writes an update audit record.
// When lockTournament is true (status transitioning to live, completed, or
// abandoned), a FOR SHARE lock is acquired on the tournament row first to
// prevent a concurrent tournament cancellation from racing with the transition.
func (r *Repository) UpdateWithAudit(ctx context.Context, p updateMatchTxParams) (*db.Match, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	if p.lockTournament {
		status, err := qtx.LockTournamentForShare(ctx, db.LockTournamentForShareParams{
			ID:             p.tournamentID,
			OrganizationID: p.organizationID,
		})
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, ErrTournamentNotFound
			}
			return nil, err
		}
		if status != db.TournamentStatusOngoing {
			return nil, ErrTournamentNotOngoing
		}
	}

	m, err := qtx.UpdateMatch(ctx, p.updateParams)
	if err != nil {
		if err == pgx.ErrNoRows {
			// The match exists (verified by GetByID in the service) but the
			// DB-level terminal-state guard rejected the write — another
			// concurrent request already transitioned the match to a terminal
			// status between the service read and this transaction.
			return nil, ErrMatchNotUpdatable
		}
		return nil, err
	}

	newData, err := matchToAuditJSON(&m)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: m.OrganizationID,
		UserID:         p.actorID,
		Action:         db.AuditActionUpdate,
		EntityType:     "matches",
		EntityID:       m.ID,
		OldData:        p.oldData,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &m, nil
}

type cancelMatchTxParams struct {
	id      pgtype.UUID
	orgID   pgtype.UUID
	actorID pgtype.UUID
	oldData []byte
}

// CancelWithAudit atomically sets the match status to cancelled and writes
// a delete audit record. Records are never hard-deleted.
func (r *Repository) CancelWithAudit(ctx context.Context, p cancelMatchTxParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	_, err = qtx.CancelMatch(ctx, db.CancelMatchParams{
		ID:             p.id,
		OrganizationID: p.orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			// The match exists (verified by GetByID in the service) but the
			// DB-level terminal-state guard rejected the write — another
			// concurrent request already transitioned the match to a terminal
			// status between the service read and this transaction.
			return ErrMatchNotUpdatable
		}
		return err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: p.orgID,
		UserID:         p.actorID,
		Action:         db.AuditActionDelete,
		EntityType:     "matches",
		EntityID:       p.id,
		OldData:        p.oldData,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func matchToAuditJSON(m *db.Match) ([]byte, error) {
	metadata := json.RawMessage("{}")
	if len(m.Metadata) > 0 {
		metadata = json.RawMessage(m.Metadata)
	}
	return json.Marshal(map[string]any{
		"id":               pgutil.UUIDToString(m.ID),
		"tournament_id":    pgutil.UUIDToString(m.TournamentID),
		"organization_id":  pgutil.UUIDToString(m.OrganizationID),
		"round_number":     m.RoundNumber,
		"round_name":       m.RoundName,
		"match_number":     m.MatchNumber,
		"home_team_id":     pgutil.UUIDToString(m.HomeTeamID),
		"away_team_id":     pgutil.UUIDToString(m.AwayTeamID),
		"home_player_id":   pgutil.UUIDToString(m.HomePlayerID),
		"away_player_id":   pgutil.UUIDToString(m.AwayPlayerID),
		"venue":            m.Venue,
		"scheduled_at":     tsForAudit(m.ScheduledAt),
		"started_at":       tsForAudit(m.StartedAt),
		"ended_at":         tsForAudit(m.EndedAt),
		"status":           string(m.Status),
		"winner_team_id":   pgutil.UUIDToString(m.WinnerTeamID),
		"winner_player_id": pgutil.UUIDToString(m.WinnerPlayerID),
		"is_walkover":      m.IsWalkover,
		"notes":            m.Notes,
		"metadata":         metadata,
		"created_at":       m.CreatedAt.Time.UTC().Format(time.RFC3339),
		"updated_at":       m.UpdatedAt.Time.UTC().Format(time.RFC3339),
	})
}

func tsForAudit(ts pgtype.Timestamptz) *string {
	if !ts.Valid {
		return nil
	}
	s := ts.Time.UTC().Format(time.RFC3339)
	return &s
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
