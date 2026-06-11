package tournament_registrations

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

// Repository provides data access for the tournament_registrations domain.
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

// GetTeamByID fetches a team scoped to an org. Returns ErrTeamNotFound if
// the team does not exist or does not belong to the org (cross-org protection).
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

// HasActiveMembers reports whether a team has at least one active member.
func (r *Repository) HasActiveMembers(ctx context.Context, teamID, orgID pgtype.UUID) (bool, error) {
	return r.queries.HasActiveMembersByTeam(ctx, db.HasActiveMembersByTeamParams{
		TeamID:         teamID,
		OrganizationID: orgID,
	})
}

// CountActiveRegistrations counts pending+approved registrations for capacity checks.
func (r *Repository) CountActiveRegistrations(ctx context.Context, tournamentID pgtype.UUID) (int64, error) {
	return r.queries.CountActiveRegistrations(ctx, tournamentID)
}

// GetPlayerByID fetches a player scoped to an org. Returns ErrPlayerNotFound if
// the player does not exist or does not belong to the org.
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

// GetByPlayer checks whether a player is already registered for a tournament.
func (r *Repository) GetByPlayer(ctx context.Context, tournamentID, playerID pgtype.UUID) (*db.TournamentRegistration, error) {
	reg, err := r.queries.GetRegistrationByPlayer(ctx, db.GetRegistrationByPlayerParams{
		TournamentID: tournamentID,
		PlayerID:     pgtype.UUID{Bytes: playerID.Bytes, Valid: true},
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // not registered — OK to proceed
		}
		return nil, err
	}
	return &reg, nil
}

// GetByTeam checks whether a team is already registered for a tournament.
func (r *Repository) GetByTeam(ctx context.Context, tournamentID, teamID pgtype.UUID) (*db.TournamentRegistration, error) {
	reg, err := r.queries.GetRegistrationByTeam(ctx, db.GetRegistrationByTeamParams{
		TournamentID: tournamentID,
		TeamID:       teamID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // not registered — OK to proceed
		}
		return nil, err
	}
	return &reg, nil
}

// GetByID fetches a registration scoped to its tournament.
func (r *Repository) GetByID(ctx context.Context, id, tournamentID pgtype.UUID) (*db.TournamentRegistration, error) {
	reg, err := r.queries.GetRegistrationByID(ctx, db.GetRegistrationByIDParams{
		ID:           id,
		TournamentID: tournamentID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrRegistrationNotFound
		}
		return nil, err
	}
	return &reg, nil
}

// List returns a paginated list of registrations for a tournament with
// participant display names joined in.
func (r *Repository) List(ctx context.Context, tournamentID pgtype.UUID, params ListParams) ([]db.ListRegistrationsByTournamentPaginatedRow, error) {
	teamFilter, playerFilter, err := parseParticipantFilters(params)
	if err != nil {
		return nil, err
	}
	return r.queries.ListRegistrationsByTournamentPaginated(ctx, db.ListRegistrationsByTournamentPaginatedParams{
		TournamentID: tournamentID,
		StatusFilter: params.StatusFilter,
		TeamFilter:   teamFilter,
		PlayerFilter: playerFilter,
		PageLimit:    params.Limit,
		PageOffset:   params.Offset,
	})
}

// Count returns the total registration count for pagination metadata.
func (r *Repository) Count(ctx context.Context, tournamentID pgtype.UUID, params ListParams) (int64, error) {
	teamFilter, playerFilter, err := parseParticipantFilters(params)
	if err != nil {
		return 0, err
	}
	return r.queries.CountRegistrationsByTournament(ctx, db.CountRegistrationsByTournamentParams{
		TournamentID: tournamentID,
		StatusFilter: params.StatusFilter,
		TeamFilter:   teamFilter,
		PlayerFilter: playerFilter,
	})
}

// parseParticipantFilters converts optional team/player filter strings to
// pgtype.UUIDs. Invalid UUIDs map to participant-not-found errors so callers
// surface a 404 rather than a 500.
func parseParticipantFilters(params ListParams) (team, player pgtype.UUID, err error) {
	if params.TeamFilter != nil && *params.TeamFilter != "" {
		team, err = pgutil.ParseUUID(*params.TeamFilter)
		if err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, ErrTeamNotFound
		}
	}
	if params.PlayerFilter != nil && *params.PlayerFilter != "" {
		player, err = pgutil.ParseUUID(*params.PlayerFilter)
		if err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, ErrPlayerNotFound
		}
	}
	return team, player, nil
}

// ── transactional writes ──────────────────────────────────────────────────────

type createRegistrationTxParams struct {
	createParams    db.CreateRegistrationParams
	actorID         pgtype.UUID
	maxParticipants *int16 // nil = no capacity limit
}

// CreateWithAudit atomically enforces capacity and inserts the registration.
//
// Capacity enforcement is performed under a tournament row lock to prevent
// concurrent over-registration. The sequence inside the transaction is:
//  1. SELECT … FOR UPDATE on the tournament row — serializes all concurrent
//     registrations for the same tournament through this exclusive lock.
//  2. COUNT active registrations — now race-free because no other transaction
//     can commit a new registration until this one releases the lock.
//  3. Reject with ErrTournamentFull if count >= max_participants.
//  4. INSERT the registration.
//  5. INSERT the audit record.
//  6. COMMIT — releases the tournament lock.
func (r *Repository) CreateWithAudit(ctx context.Context, p createRegistrationTxParams) (*db.TournamentRegistration, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	// Capacity enforcement is performed under a tournament row lock to prevent
	// concurrent over-registration. Lock first, then count, then insert — all
	// within the same transaction so no other session can insert between steps.
	if p.maxParticipants != nil {
		if _, err := qtx.LockTournamentForUpdate(ctx, p.createParams.TournamentID); err != nil {
			if err == pgx.ErrNoRows {
				return nil, ErrTournamentNotFound
			}
			return nil, err
		}

		count, err := qtx.CountActiveRegistrations(ctx, p.createParams.TournamentID)
		if err != nil {
			return nil, err
		}
		if count >= int64(*p.maxParticipants) {
			return nil, ErrTournamentFull
		}
	}

	reg, err := qtx.CreateRegistration(ctx, p.createParams)
	if err != nil {
		if pgutil.IsUniqueViolation(err, "uq_treg_tournament_team") {
			return nil, ErrAlreadyRegistered
		}
		return nil, err
	}

	newData, err := registrationToAuditJSON(&reg)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: reg.OrganizationID,
		UserID:         p.actorID,
		Action:         db.AuditActionCreate,
		EntityType:     "tournament_registrations",
		EntityID:       reg.ID,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &reg, nil
}

type updateRegistrationTxParams struct {
	updateParams db.UpdateRegistrationParams
	actorID      pgtype.UUID
	orgID        pgtype.UUID
	oldData      []byte
	// previousStatus is the status observed by the service before the transaction.
	// Used to detect status changes for outbox entry creation.
	previousStatus db.RegistrationStatus
}

// UpdateWithAudit atomically updates a registration and writes an update audit record.
// Writes a registration status outbox entry when the status transitions.
func (r *Repository) UpdateWithAudit(ctx context.Context, p updateRegistrationTxParams) (*db.TournamentRegistration, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	reg, err := qtx.UpdateRegistration(ctx, p.updateParams)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrRegistrationNotFound
		}
		return nil, err
	}

	newData, err := registrationToAuditJSON(&reg)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: p.orgID,
		UserID:         p.actorID,
		Action:         db.AuditActionUpdate,
		EntityType:     "tournament_registrations",
		EntityID:       reg.ID,
		OldData:        p.oldData,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	// Write outbox entry when the status changed to a notifiable state.
	if reg.Status != p.previousStatus {
		if eventType, ok := registrationStatusToEventType(reg.Status); ok {
			if err := trigger.WriteOutboxEntry(ctx, qtx, trigger.OutboxParams{
				OrganizationID: p.orgID,
				EventType:      eventType,
				ActorID:        p.actorID,
				EntityType:     "tournament_registrations",
				EntityID:       reg.ID,
				Payload: map[string]any{
					"registration_id": pgutil.UUIDToString(reg.ID),
					"tournament_id":   pgutil.UUIDToString(reg.TournamentID),
					"previous_status": string(p.previousStatus),
					"new_status":      string(reg.Status),
				},
			}); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &reg, nil
}

type withdrawRegistrationTxParams struct {
	id           pgtype.UUID
	tournamentID pgtype.UUID
	orgID        pgtype.UUID
	actorID      pgtype.UUID
	oldData      []byte
	// previousStatus is the status observed by the service before the transaction.
	previousStatus db.RegistrationStatus
}

// WithdrawWithAudit atomically sets the registration to withdrawn and writes
// a delete audit record. Records are never hard-deleted.
// Writes a registration_withdrawn outbox entry for notification fan-out.
func (r *Repository) WithdrawWithAudit(ctx context.Context, p withdrawRegistrationTxParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	withdrawn, err := qtx.WithdrawRegistration(ctx, db.WithdrawRegistrationParams{
		ID:           p.id,
		TournamentID: p.tournamentID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrRegistrationNotFound
		}
		return err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: p.orgID,
		UserID:         p.actorID,
		Action:         db.AuditActionDelete,
		EntityType:     "tournament_registrations",
		EntityID:       p.id,
		OldData:        p.oldData,
	}); err != nil {
		return err
	}

	// Write outbox entry for notification fan-out.
	if err := trigger.WriteOutboxEntry(ctx, qtx, trigger.OutboxParams{
		OrganizationID: p.orgID,
		EventType:      db.NotificationEventTypeRegistrationWithdrawn,
		ActorID:        p.actorID,
		EntityType:     "tournament_registrations",
		EntityID:       p.id,
		Payload: map[string]any{
			"registration_id": pgutil.UUIDToString(withdrawn.ID),
			"tournament_id":   pgutil.UUIDToString(withdrawn.TournamentID),
			"previous_status": string(p.previousStatus),
			"new_status":      "withdrawn",
		},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// registrationStatusToEventType maps registration status values to outbox event types.
// Returns (eventType, true) for notifiable status values.
func registrationStatusToEventType(status db.RegistrationStatus) (db.NotificationEventType, bool) {
	switch status {
	case db.RegistrationStatusApproved:
		return db.NotificationEventTypeRegistrationApproved, true
	case db.RegistrationStatusRejected:
		return db.NotificationEventTypeRegistrationRejected, true
	case db.RegistrationStatusWithdrawn:
		return db.NotificationEventTypeRegistrationWithdrawn, true
	}
	return "", false
}

// ── helpers ───────────────────────────────────────────────────────────────────

func registrationToAuditJSON(r *db.TournamentRegistration) ([]byte, error) {
	var teamID, playerID *string
	if r.TeamID.Valid {
		s := pgutil.UUIDToString(r.TeamID)
		teamID = &s
	}
	if r.PlayerID.Valid {
		s := pgutil.UUIDToString(r.PlayerID)
		playerID = &s
	}
	var approvedAt *string
	if r.ApprovedAt.Valid {
		s := r.ApprovedAt.Time.UTC().Format(time.RFC3339)
		approvedAt = &s
	}
	return json.Marshal(map[string]any{
		"id":              pgutil.UUIDToString(r.ID),
		"tournament_id":   pgutil.UUIDToString(r.TournamentID),
		"organization_id": pgutil.UUIDToString(r.OrganizationID),
		"team_id":         teamID,
		"player_id":       playerID,
		"seed_number":     r.SeedNumber,
		"status":          string(r.Status),
		"registered_at":   r.RegisteredAt.Time.UTC().Format(time.RFC3339),
		"approved_at":     approvedAt,
		"notes":           r.Notes,
		"created_at":      r.CreatedAt.Time.UTC().Format(time.RFC3339),
		"updated_at":      r.UpdatedAt.Time.UTC().Format(time.RFC3339),
	})
}
