package matches

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
	"github.com/4yushraman-jpg/playarena/internal/scoring"
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

// CountFeedersForSlot reports how many active matches (other than excludeID)
// already advance into the given successor slot. Used to reject linking two
// feeders into one slot. excludeID may be a zero UUID on create.
func (r *Repository) CountFeedersForSlot(ctx context.Context, nextMatchID pgtype.UUID, slot *int16, orgID, excludeID pgtype.UUID) (int64, error) {
	return r.queries.CountMatchesFeedingSlot(ctx, db.CountMatchesFeedingSlotParams{
		NextMatchID:    nextMatchID,
		NextMatchSlot:  slot,
		OrganizationID: orgID,
		Column4:        excludeID,
	})
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

// GetEffectiveEventsForScore returns all effective (non-cancelled) match events
// in sequence order for the scoring engine.  No pagination is applied: the
// engine requires the complete effective timeline to produce a correct score.
func (r *Repository) GetEffectiveEventsForScore(ctx context.Context, matchID, orgID pgtype.UUID) ([]db.MatchEvent, error) {
	return r.queries.GetEffectiveMatchEventsForScore(ctx, db.GetEffectiveMatchEventsForScoreParams{
		MatchID:        matchID,
		OrganizationID: orgID,
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
//  5. Writes a match_created outbox entry for notification fan-out.
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

	// Write outbox entry for notification fan-out (post-commit drain).
	if err := trigger.WriteOutboxEntry(ctx, qtx, trigger.OutboxParams{
		OrganizationID: m.OrganizationID,
		EventType:      db.NotificationEventTypeMatchCreated,
		ActorID:        p.actorID,
		EntityType:     "matches",
		EntityID:       m.ID,
		Payload: map[string]any{
			"match_id":      pgutil.UUIDToString(m.ID),
			"tournament_id": pgutil.UUIDToString(m.TournamentID),
			"status":        string(m.Status),
		},
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
	// isCompletion is true when the transition is live → completed.
	isCompletion bool
	// isWalkover is passed from the service so the repository can exempt
	// walkover completions from the winner-vs-score consistency check.
	isWalkover bool
	// previousStatus is the match status observed by the service before the
	// transaction. Used as the CAS guard in UpdateMatch.
	previousStatus db.MatchStatus
}

// UpdateWithAudit atomically updates a match and writes an update audit record.
// When lockTournament is true (status transitioning to live, completed, or
// abandoned), a FOR SHARE lock is acquired on the tournament row first to
// prevent a concurrent tournament cancellation from racing with the transition.
// Uses compare-and-swap (AND status = previousStatus) to prevent split-brain
// on concurrent PATCH requests.
// Writes a status-change outbox entry when the status transitions.
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

	// ── Score snapshot for live → completed transitions ─────────────────────
	if p.isCompletion {
		locked, err := qtx.LockMatchForUpdate(ctx, db.LockMatchForUpdateParams{
			ID:             p.updateParams.ID,
			OrganizationID: p.organizationID,
		})
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, ErrMatchNotFound
			}
			return nil, err
		}
		if locked.Status != db.MatchStatusLive {
			return nil, ErrMatchNotUpdatable
		}

		events, err := qtx.GetEffectiveMatchEventsForScore(ctx, db.GetEffectiveMatchEventsForScoreParams{
			MatchID:        p.updateParams.ID,
			OrganizationID: p.organizationID,
		})
		if err != nil {
			return nil, err
		}

		matchForEngine := &db.Match{
			ID:           locked.ID,
			HomeTeamID:   locked.HomeTeamID,
			AwayTeamID:   locked.AwayTeamID,
			HomePlayerID: locked.HomePlayerID,
			AwayPlayerID: locked.AwayPlayerID,
		}
		result := scoring.NewScoreEngine().Compute(matchForEngine, events)

		if err := validateWinnerVsScore(p, locked, result.HomeScore, result.AwayScore); err != nil {
			return nil, err
		}

		p.updateParams.HomeScore = int32(result.HomeScore)
		p.updateParams.AwayScore = int32(result.AwayScore)
	}

	m, err := qtx.UpdateMatch(ctx, p.updateParams)
	if err != nil {
		if err == pgx.ErrNoRows {
			// CAS failed: the match status changed between the service read and
			// this transaction (concurrent request or terminal-state race).
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

	// Write outbox entry when the status changed.
	if eventType, ok := matchStatusToEventType(m.Status); ok && m.Status != p.previousStatus {
		if err := trigger.WriteOutboxEntry(ctx, qtx, trigger.OutboxParams{
			OrganizationID: m.OrganizationID,
			EventType:      eventType,
			ActorID:        p.actorID,
			EntityType:     "matches",
			EntityID:       m.ID,
			Payload: map[string]any{
				"match_id":        pgutil.UUIDToString(m.ID),
				"tournament_id":   pgutil.UUIDToString(m.TournamentID),
				"previous_status": string(p.previousStatus),
				"new_status":      string(m.Status),
			},
		}); err != nil {
			return nil, err
		}
	}

	// Bracket progression (FE-8B): on completion, advance the winner into the
	// linked successor — in this same transaction, so completion + propagation
	// are atomic. A blocked/inconsistent successor rolls the completion back.
	if m.Status == db.MatchStatusCompleted {
		if err := propagateWinner(ctx, qtx, &m); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &m, nil
}

type cancelMatchTxParams struct {
	id             pgtype.UUID
	orgID          pgtype.UUID
	actorID        pgtype.UUID
	oldData        []byte
	previousStatus db.MatchStatus
}

// CancelWithAudit atomically sets the match status to cancelled and writes
// a delete audit record. Records are never hard-deleted.
// Uses CAS (AND status = previousStatus) to guard against concurrent transitions.
// Writes a match_cancelled outbox entry for notification fan-out.
func (r *Repository) CancelWithAudit(ctx context.Context, p cancelMatchTxParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	cancelled, err := qtx.CancelMatch(ctx, db.CancelMatchParams{
		ID:             p.id,
		OrganizationID: p.orgID,
		Status:         p.previousStatus, // CAS guard: $3 = previous_status
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			// CAS failed: status changed between service read and this transaction.
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

	// Write outbox entry for notification fan-out.
	if err := trigger.WriteOutboxEntry(ctx, qtx, trigger.OutboxParams{
		OrganizationID: p.orgID,
		EventType:      db.NotificationEventTypeMatchCancelled,
		ActorID:        p.actorID,
		EntityType:     "matches",
		EntityID:       p.id,
		Payload: map[string]any{
			"match_id":        pgutil.UUIDToString(cancelled.ID),
			"tournament_id":   pgutil.UUIDToString(cancelled.TournamentID),
			"previous_status": string(p.previousStatus),
			"new_status":      "cancelled",
		},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

type walkoverMatchTxParams struct {
	id             pgtype.UUID
	orgID          pgtype.UUID
	tournamentID   pgtype.UUID
	winnerTeamID   pgtype.UUID
	winnerPlayerID pgtype.UUID
	reason         *string
	actorID        pgtype.UUID
	oldData        []byte
	previousStatus db.MatchStatus
}

// WalkoverWithAudit atomically awards a walkover, writes an update audit record,
// and enqueues a match-conclusion outbox entry.
//
// A FOR SHARE lock on the tournament row guards against a concurrent tournament
// cancellation; the tournament must be ongoing. The SetMatchWalkover CAS guard
// (status = previousStatus) makes concurrent walkover/transition attempts safe:
// only the first commits; later attempts match 0 rows → ErrMatchNotUpdatable.
// This is the same race protection used by UpdateWithAudit and CancelWithAudit.
func (r *Repository) WalkoverWithAudit(ctx context.Context, p walkoverMatchTxParams) (*db.Match, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	// Lock the tournament row: a walkover advances a match's lifecycle, so the
	// parent tournament must still be ongoing and must not be racing a cancel.
	status, err := qtx.LockTournamentForShare(ctx, db.LockTournamentForShareParams{
		ID:             p.tournamentID,
		OrganizationID: p.orgID,
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

	m, err := qtx.SetMatchWalkover(ctx, db.SetMatchWalkoverParams{
		ID:             p.id,
		OrganizationID: p.orgID,
		WinnerTeamID:   p.winnerTeamID,
		WinnerPlayerID: p.winnerPlayerID,
		Notes:          p.reason,
		Status:         p.previousStatus, // CAS guard: $6 = previous_status
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			// CAS failed: status changed between the service read and this tx.
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

	// A walkover is a match conclusion. There is no dedicated walkover event in
	// the notification enum, so reuse match_completed; the payload carries
	// is_walkover and the walkover status so consumers can distinguish it.
	if err := trigger.WriteOutboxEntry(ctx, qtx, trigger.OutboxParams{
		OrganizationID: m.OrganizationID,
		EventType:      db.NotificationEventTypeMatchCompleted,
		ActorID:        p.actorID,
		EntityType:     "matches",
		EntityID:       m.ID,
		Payload: map[string]any{
			"match_id":        pgutil.UUIDToString(m.ID),
			"tournament_id":   pgutil.UUIDToString(m.TournamentID),
			"previous_status": string(p.previousStatus),
			"new_status":      string(m.Status),
			"is_walkover":     true,
		},
	}); err != nil {
		return nil, err
	}

	// Bracket progression (FE-8B): a walkover concludes with a winner and
	// propagates identically to a scored completion, in the same transaction.
	if err := propagateWinner(ctx, qtx, &m); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &m, nil
}

// propagateWinner advances a concluded feeder's winner into its linked
// successor's designated slot, INSIDE the caller's transaction. It is invoked
// from the completion and walkover transactions so propagation is atomic with
// the result that triggered it — a partial failure can never leave a winner
// recorded but unpropagated.
//
// Invariants enforced:
//   - I2 (no double propagation): the slot is fixed per feeder (next_match_slot),
//     so re-running writes the SAME slot idempotently — never a second slot.
//   - I3 (no stale propagation): the successor is locked FOR UPDATE and must
//     still be 'scheduled'; if it has started or concluded, the whole
//     transaction is aborted with ErrDownstreamLocked.
//   - I5 (bracket integrity): the successor must belong to the same tournament.
//
// No-op when the feeder has no link or no winner (e.g. a drawn completion).
func propagateWinner(ctx context.Context, qtx *db.Queries, feeder *db.Match) error {
	if !feeder.NextMatchID.Valid || feeder.NextMatchSlot == nil {
		return nil
	}
	winnerTeam := feeder.WinnerTeamID
	winnerPlayer := feeder.WinnerPlayerID
	if !winnerTeam.Valid && !winnerPlayer.Valid {
		return nil // no winner to advance (draw)
	}

	succ, err := qtx.LockMatchForProgression(ctx, db.LockMatchForProgressionParams{
		ID:             feeder.NextMatchID,
		OrganizationID: feeder.OrganizationID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNextMatchNotFound
		}
		return err
	}
	if succ.TournamentID.Bytes != feeder.TournamentID.Bytes {
		return ErrBracketInconsistent
	}
	// I3: a winner may only flow into a still-scheduled successor.
	if succ.Status != db.MatchStatusScheduled {
		return ErrDownstreamLocked
	}

	// Preserve the successor's other slot; set only this feeder's fixed slot.
	params := db.SetMatchParticipantsParams{
		ID:             succ.ID,
		OrganizationID: succ.OrganizationID,
		HomeTeamID:     succ.HomeTeamID,
		AwayTeamID:     succ.AwayTeamID,
		HomePlayerID:   succ.HomePlayerID,
		AwayPlayerID:   succ.AwayPlayerID,
	}
	switch *feeder.NextMatchSlot {
	case 1: // home
		params.HomeTeamID = winnerTeam
		params.HomePlayerID = winnerPlayer
	case 2: // away
		params.AwayTeamID = winnerTeam
		params.AwayPlayerID = winnerPlayer
	default:
		return ErrInvalidNextSlot
	}

	n, err := qtx.SetMatchParticipants(ctx, params)
	if err != nil {
		return err
	}
	if n == 0 {
		// The row left 'scheduled' between lock and write — impossible under the
		// FOR UPDATE lock, but treated as a block rather than a silent drop.
		return ErrDownstreamLocked
	}
	return nil
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
		"home_score":       m.HomeScore,
		"away_score":       m.AwayScore,
		"notes":            m.Notes,
		"next_match_id":    pgutil.UUIDToString(m.NextMatchID),
		"next_match_slot":  m.NextMatchSlot,
		"group_label":      m.GroupLabel,
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

// matchStatusToEventType maps match status transitions to outbox event types.
// Returns (eventType, true) when the status has a corresponding event type.
func matchStatusToEventType(status db.MatchStatus) (db.NotificationEventType, bool) {
	switch status {
	case db.MatchStatusLive:
		return db.NotificationEventTypeMatchStarted, true
	case db.MatchStatusCompleted:
		return db.NotificationEventTypeMatchCompleted, true
	case db.MatchStatusCancelled:
		return db.NotificationEventTypeMatchCancelled, true
	case db.MatchStatusAbandoned:
		return db.NotificationEventTypeMatchAbandoned, true
	}
	return "", false
}

// validateWinnerVsScore verifies that the declared winner is consistent with
// the computed final score.  Walkovers are always exempt: the score is 0-0
// by convention but a winner must be set.
//
// Rules (non-walkover):
//   - homeScore > awayScore  → winner must be the home participant
//   - awayScore > homeScore  → winner must be the away participant
//   - homeScore == awayScore → winner must be absent (draw)
func validateWinnerVsScore(
	p updateMatchTxParams,
	locked db.LockMatchForUpdateRow,
	homeScore, awayScore int,
) error {
	if p.isWalkover {
		return nil
	}

	var winnerUID pgtype.UUID
	if p.updateParams.WinnerTeamID.Valid {
		winnerUID = p.updateParams.WinnerTeamID
	} else if p.updateParams.WinnerPlayerID.Valid {
		winnerUID = p.updateParams.WinnerPlayerID
	}

	var homeUID pgtype.UUID
	if locked.HomeTeamID.Valid {
		homeUID = locked.HomeTeamID
	} else if locked.HomePlayerID.Valid {
		homeUID = locked.HomePlayerID
	}

	var awayUID pgtype.UUID
	if locked.AwayTeamID.Valid {
		awayUID = locked.AwayTeamID
	} else if locked.AwayPlayerID.Valid {
		awayUID = locked.AwayPlayerID
	}

	switch {
	case homeScore > awayScore:
		if !winnerUID.Valid || winnerUID.Bytes != homeUID.Bytes {
			return ErrWinnerScoreMismatch
		}
	case awayScore > homeScore:
		if !winnerUID.Valid || winnerUID.Bytes != awayUID.Bytes {
			return ErrWinnerScoreMismatch
		}
	default: // equal scores → draw
		if winnerUID.Valid {
			return ErrWinnerScoreMismatch
		}
	}
	return nil
}
