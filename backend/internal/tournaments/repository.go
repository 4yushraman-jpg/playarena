package tournaments

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/notifications/trigger"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Repository provides data access for the tournaments domain.
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

// GetByID fetches a single tournament by UUID within an organization.
// No status filter: cancelled (soft-deleted) tournaments are intentionally
// returned so that future registration and match references remain resolvable.
func (r *Repository) GetByID(ctx context.Context, id, orgID pgtype.UUID) (*db.Tournament, error) {
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

// GetBySlug fetches a tournament by slug within an organization.
func (r *Repository) GetBySlug(ctx context.Context, slug string, orgID pgtype.UUID) (*db.Tournament, error) {
	t, err := r.queries.GetTournamentBySlug(ctx, db.GetTournamentBySlugParams{
		Slug:           slug,
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

// List returns a paginated page of non-cancelled tournaments for an org.
func (r *Repository) List(ctx context.Context, orgID pgtype.UUID, params ListParams) ([]db.Tournament, error) {
	return r.queries.ListTournamentsPaginated(ctx, db.ListTournamentsPaginatedParams{
		OrganizationID: orgID,
		StatusFilter:   params.StatusFilter,
		SearchQuery:    params.Search,
		PageLimit:      params.Limit,
		PageOffset:     params.Offset,
	})
}

// Count returns the total count of non-cancelled tournaments matching the filters.
func (r *Repository) Count(ctx context.Context, orgID pgtype.UUID, params ListParams) (int64, error) {
	return r.queries.CountTournamentsByOrganization(ctx, db.CountTournamentsByOrganizationParams{
		OrganizationID: orgID,
		StatusFilter:   params.StatusFilter,
		SearchQuery:    params.Search,
	})
}

// GetCompletedMatchesForStandings returns all completed matches for a
// tournament in creation order.  Both organization_id and tournament_id are
// required for multi-tenant isolation.
func (r *Repository) GetCompletedMatchesForStandings(ctx context.Context, tournamentID, orgID pgtype.UUID) ([]db.ListCompletedMatchesByTournamentRow, error) {
	return r.queries.ListCompletedMatchesByTournament(ctx, db.ListCompletedMatchesByTournamentParams{
		TournamentID:   tournamentID,
		OrganizationID: orgID,
	})
}

// CountRegistrationsByStatus returns per-status registration counts for a set
// of tournaments in a single grouped query.
func (r *Repository) CountRegistrationsByStatus(ctx context.Context, tournamentIDs []pgtype.UUID) ([]db.CountRegistrationsByStatusForTournamentsRow, error) {
	return r.queries.CountRegistrationsByStatusForTournaments(ctx, tournamentIDs)
}

// GetTeamNamesByIDs batch-resolves team display names.
func (r *Repository) GetTeamNamesByIDs(ctx context.Context, ids []pgtype.UUID) ([]db.ListTeamNamesByIDsRow, error) {
	return r.queries.ListTeamNamesByIDs(ctx, ids)
}

// GetPlayerNamesByIDs batch-resolves player display names.
func (r *Repository) GetPlayerNamesByIDs(ctx context.Context, ids []pgtype.UUID) ([]db.ListPlayerNamesByIDsRow, error) {
	return r.queries.ListPlayerNamesByIDs(ctx, ids)
}

// GetRegistrationsForStandings returns all approved registrations for a
// tournament, ordered by registered_at ASC for deterministic tiebreaking.
// No organization_id filter: registrations may come from any org in
// multi-club tournaments; the host tournament is already verified by the
// caller via (tournamentID, hostOrgID) before this method is invoked.
func (r *Repository) GetRegistrationsForStandings(ctx context.Context, tournamentID pgtype.UUID) ([]db.ListApprovedRegistrationsForStandingsRow, error) {
	return r.queries.ListApprovedRegistrationsForStandings(ctx, tournamentID)
}

// ── transactional writes ──────────────────────────────────────────────────────

type createTournamentTxParams struct {
	createParams db.CreateTournamentParams
	actorID      pgtype.UUID
}

// CreateWithAudit atomically inserts the tournament and writes a create audit record.
// newData is derived from the actual DB-returned row.
func (r *Repository) CreateWithAudit(ctx context.Context, p createTournamentTxParams) (*db.Tournament, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	t, err := qtx.CreateTournament(ctx, p.createParams)
	if err != nil {
		if pgutil.IsUniqueViolation(err, "uq_tournaments_org_slug") {
			return nil, ErrSlugAlreadyTaken
		}
		return nil, err
	}

	newData, err := tournamentToAuditJSON(&t)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: t.OrganizationID,
		UserID:         p.actorID,
		Action:         db.AuditActionCreate,
		EntityType:     "tournaments",
		EntityID:       t.ID,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &t, nil
}

type updateTournamentTxParams struct {
	updateParams db.UpdateTournamentParams
	actorID      pgtype.UUID
	oldData      []byte
	// previousStatus is the status observed by the service before the transaction.
	// Used to detect status changes and write outbox entries only when the status
	// actually transitions.
	previousStatus db.TournamentStatus
}

// UpdateWithAudit atomically updates the tournament and writes an update audit record.
// Writes a tournament_status_changed outbox entry when the status transitions.
func (r *Repository) UpdateWithAudit(ctx context.Context, p updateTournamentTxParams) (*db.Tournament, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	t, err := qtx.UpdateTournament(ctx, p.updateParams)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrTournamentNotFound
		}
		return nil, err
	}

	newData, err := tournamentToAuditJSON(&t)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: t.OrganizationID,
		UserID:         p.actorID,
		Action:         db.AuditActionUpdate,
		EntityType:     "tournaments",
		EntityID:       t.ID,
		OldData:        p.oldData,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	// Write outbox entry when the status changed.
	if t.Status != p.previousStatus {
		if err := trigger.WriteOutboxEntry(ctx, qtx, trigger.OutboxParams{
			OrganizationID: t.OrganizationID,
			EventType:      db.NotificationEventTypeTournamentStatusChanged,
			ActorID:        p.actorID,
			EntityType:     "tournaments",
			EntityID:       t.ID,
			Payload: map[string]any{
				"tournament_id":   pgutil.UUIDToString(t.ID),
				"previous_status": string(p.previousStatus),
				"new_status":      string(t.Status),
			},
		}); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &t, nil
}

type cancelTournamentTxParams struct {
	id             pgtype.UUID
	orgID          pgtype.UUID
	actorID        pgtype.UUID
	oldData        []byte
	previousStatus db.TournamentStatus
}

// CancelWithAudit atomically sets the tournament status to cancelled and writes
// a delete audit record. Records are never hard-deleted.
// Writes a tournament_status_changed outbox entry for notification fan-out.
func (r *Repository) CancelWithAudit(ctx context.Context, p cancelTournamentTxParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	_, err = qtx.CancelTournament(ctx, db.CancelTournamentParams{
		ID:             p.id,
		OrganizationID: p.orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrTournamentNotFound
		}
		return err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: p.orgID,
		UserID:         p.actorID,
		Action:         db.AuditActionDelete,
		EntityType:     "tournaments",
		EntityID:       p.id,
		OldData:        p.oldData,
	}); err != nil {
		return err
	}

	// Write outbox entry for notification fan-out.
	if err := trigger.WriteOutboxEntry(ctx, qtx, trigger.OutboxParams{
		OrganizationID: p.orgID,
		EventType:      db.NotificationEventTypeTournamentStatusChanged,
		ActorID:        p.actorID,
		EntityType:     "tournaments",
		EntityID:       p.id,
		Payload: map[string]any{
			"tournament_id":   pgutil.UUIDToString(p.id),
			"previous_status": string(p.previousStatus),
			"new_status":      "cancelled",
		},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func tournamentToAuditJSON(t *db.Tournament) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":               pgutil.UUIDToString(t.ID),
		"organization_id":  pgutil.UUIDToString(t.OrganizationID),
		"name":             t.Name,
		"slug":             t.Slug,
		"description":      t.Description,
		"sport":            t.Sport,
		"format":           string(t.Format),
		"participant_type": string(t.ParticipantType),
		"status":           string(t.Status),
		"banner_url":       t.BannerUrl,
		"currency":         t.Currency,
		"max_participants": t.MaxParticipants,
		"min_participants": t.MinParticipants,
		"venue":            t.Venue,
		"city":             t.City,
		"country":          t.Country,
		"created_at":       t.CreatedAt.Time.UTC().Format(time.RFC3339),
		"updated_at":       t.UpdatedAt.Time.UTC().Format(time.RFC3339),
	})
}
