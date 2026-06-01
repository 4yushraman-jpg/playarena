package match_events

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

// Repository provides data access for the match_events domain.
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

// GetMatchByID fetches a match scoped to its org. Used to confirm the match
// exists and belongs to the org before entering a transaction.
func (r *Repository) GetMatchByID(ctx context.Context, id, orgID pgtype.UUID) (*db.Match, error) {
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

// GetByID fetches a single match event scoped to both its match and
// organization. The match_id scope enforces the URL resource hierarchy:
// an event from match B cannot be retrieved through a match A URL.
func (r *Repository) GetByID(ctx context.Context, id, matchID, orgID pgtype.UUID) (*db.MatchEvent, error) {
	e, err := r.queries.GetMatchEventByMatchAndID(ctx, db.GetMatchEventByMatchAndIDParams{
		ID:             id,
		MatchID:        matchID,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrEventNotFound
		}
		return nil, err
	}
	return &e, nil
}

// List returns a paginated raw or effective event timeline for a match.
func (r *Repository) List(ctx context.Context, matchID, orgID pgtype.UUID, params ListParams) ([]db.MatchEvent, error) {
	if params.EffectiveOnly {
		return r.queries.ListEffectiveMatchEventsByMatch(ctx, db.ListEffectiveMatchEventsByMatchParams{
			MatchID:        matchID,
			OrganizationID: orgID,
			PageLimit:      params.Limit,
			PageOffset:     params.Offset,
		})
	}
	return r.queries.ListMatchEventsByMatch(ctx, db.ListMatchEventsByMatchParams{
		MatchID:        matchID,
		OrganizationID: orgID,
		PageLimit:      params.Limit,
		PageOffset:     params.Offset,
	})
}

// Count returns the total event count matching the same filters as List.
func (r *Repository) Count(ctx context.Context, matchID, orgID pgtype.UUID, params ListParams) (int64, error) {
	if params.EffectiveOnly {
		return r.queries.CountEffectiveMatchEventsByMatch(ctx, db.CountEffectiveMatchEventsByMatchParams{
			MatchID:        matchID,
			OrganizationID: orgID,
		})
	}
	return r.queries.CountMatchEventsByMatch(ctx, db.CountMatchEventsByMatchParams{
		MatchID:        matchID,
		OrganizationID: orgID,
	})
}

// ── transactional writes ──────────────────────────────────────────────────────

type createEventTxParams struct {
	matchID        pgtype.UUID
	organizationID pgtype.UUID
	eventType      db.MatchEventType
	teamID         pgtype.UUID
	playerID       pgtype.UUID
	period         *int16
	clockSeconds   *int32
	payload        []byte
	recordedBy     pgtype.UUID
	recordedAt     pgtype.Timestamptz
	cancelsEventID pgtype.UUID
	actorID        pgtype.UUID
}

// CreateWithAudit atomically records a match event. Inside the transaction:
//  1. SELECT … FOR UPDATE on the match row — serialises all concurrent event
//     inserts for the same match; also validates status and participants.
//  2. Match must be live.
//  3. team_id (if set) must be a match participant.
//  4. player_id (if set) must be a match participant or on a participating team.
//  5. If both team_id and player_id: player must belong to the stated team.
//  6. Lifecycle event uniqueness (match_started, match_ended).
//  7. score_correction validation (target existence, same match, not a chain, not already cancelled).
//  8. MAX(sequence_number) + 1 — race-free under the match row lock.
//  9. INSERT the event.
//
// 10. INSERT a create audit record.
func (r *Repository) CreateWithAudit(ctx context.Context, p createEventTxParams) (*db.MatchEvent, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	// ── 1. Lock match row ─────────────────────────────────────────────────────
	// FOR UPDATE serialises all concurrent event inserts for this match.
	// The lock is held until the transaction commits, making steps 2–9 atomic.
	matchRow, err := qtx.LockMatchForUpdate(ctx, db.LockMatchForUpdateParams{
		ID:             p.matchID,
		OrganizationID: p.organizationID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrMatchNotFound
		}
		return nil, err
	}

	// ── 2. Match must be live ─────────────────────────────────────────────────
	if matchRow.Status != db.MatchStatusLive {
		return nil, ErrMatchNotLive
	}

	// ── 3. Team participant validation ────────────────────────────────────────
	if p.teamID.Valid {
		if !uuidEquals(p.teamID, matchRow.HomeTeamID) && !uuidEquals(p.teamID, matchRow.AwayTeamID) {
			return nil, ErrTeamNotParticipant
		}
	}

	// ── 4. Player participant validation ──────────────────────────────────────
	if p.playerID.Valid {
		if matchRow.HomePlayerID.Valid || matchRow.AwayPlayerID.Valid {
			// Individual-format match: player must be home or away player.
			if !uuidEquals(p.playerID, matchRow.HomePlayerID) &&
				!uuidEquals(p.playerID, matchRow.AwayPlayerID) {
				return nil, ErrPlayerNotParticipant
			}
		} else if matchRow.HomeTeamID.Valid || matchRow.AwayTeamID.Valid {
			// Team-format match: player must have an active membership on a
			// participating team. The FOR UPDATE lock guarantees that team
			// membership state cannot change between this check and the INSERT.
			onTeam, err := qtx.IsPlayerOnParticipatingTeam(ctx, db.IsPlayerOnParticipatingTeamParams{
				PlayerID:   p.playerID,
				HomeTeamID: matchRow.HomeTeamID,
				AwayTeamID: matchRow.AwayTeamID,
			})
			if err != nil {
				return nil, err
			}
			if !onTeam {
				return nil, ErrPlayerNotParticipant
			}
		}
	}

	// ── 5. Player must belong to the stated team (when both are provided) ─────
	if p.teamID.Valid && p.playerID.Valid {
		onTeam, err := qtx.IsPlayerOnTeam(ctx, db.IsPlayerOnTeamParams{
			PlayerID: p.playerID,
			TeamID:   p.teamID,
		})
		if err != nil {
			return nil, err
		}
		if !onTeam {
			return nil, ErrPlayerNotOnTeam
		}
	}

	// ── 6. Lifecycle event uniqueness ─────────────────────────────────────────
	if p.eventType == db.MatchEventTypeMatchStarted || p.eventType == db.MatchEventTypeMatchEnded {
		count, err := qtx.CountMatchEventsByType(ctx, db.CountMatchEventsByTypeParams{
			MatchID:   p.matchID,
			EventType: p.eventType,
		})
		if err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, ErrDuplicateLifecycleEvent
		}
	}

	// ── 7. Score correction validation ────────────────────────────────────────
	if p.eventType == db.MatchEventTypeScoreCorrection {
		// DB constraint chk_match_events_correction_requires_target also enforces
		// this, but we check here for a clean 422 before reaching the INSERT.
		if !p.cancelsEventID.Valid {
			return nil, ErrCancelsEventRequired
		}

		// Target event must exist.
		target, err := qtx.GetMatchEventForCorrection(ctx, p.cancelsEventID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil, ErrCancelsEventNotFound
			}
			return nil, err
		}

		// Target must be in the same match (implicitly prevents cross-org reference).
		if !uuidEquals(target.MatchID, p.matchID) {
			return nil, ErrCancelsEventCrossMatch
		}

		// Target must not be another score_correction (no correction chains).
		if target.EventType == db.MatchEventTypeScoreCorrection {
			return nil, ErrCannotCancelCorrection
		}

		// Target must not already be cancelled by a prior score_correction.
		_, err = qtx.GetEventCancellation(ctx, db.GetEventCancellationParams{
			MatchID:        p.matchID,
			CancelsEventID: p.cancelsEventID,
		})
		if err == nil {
			// Row found — the event is already cancelled.
			return nil, ErrEventAlreadyCancelled
		}
		if err != pgx.ErrNoRows {
			return nil, err
		}
		// pgx.ErrNoRows → target not yet cancelled; safe to proceed.
	}

	// ── 8. Compute next sequence number ──────────────────────────────────────
	// Safe because the FOR UPDATE lock on the match row blocks any concurrent
	// INSERT that would also need to compute MAX(sequence_number)+1.
	maxSeq, err := qtx.GetMaxSequenceNumber(ctx, p.matchID)
	if err != nil {
		return nil, err
	}
	nextSeq := maxSeq + 1

	// ── 9. Insert the event ───────────────────────────────────────────────────
	event, err := qtx.CreateMatchEvent(ctx, db.CreateMatchEventParams{
		MatchID:        p.matchID,
		OrganizationID: p.organizationID,
		SequenceNumber: nextSeq,
		EventType:      p.eventType,
		TeamID:         p.teamID,
		PlayerID:       p.playerID,
		Period:         p.period,
		ClockSeconds:   p.clockSeconds,
		Payload:        p.payload,
		RecordedBy:     p.recordedBy,
		RecordedAt:     p.recordedAt,
		CancelsEventID: p.cancelsEventID,
	})
	if err != nil {
		return nil, err
	}

	// ── 10. Audit log ─────────────────────────────────────────────────────────
	newData, err := eventToAuditJSON(&event)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: event.OrganizationID,
		UserID:         p.actorID,
		Action:         db.AuditActionCreate,
		EntityType:     "match_events",
		EntityID:       event.ID,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &event, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func eventToAuditJSON(e *db.MatchEvent) ([]byte, error) {
	payload := json.RawMessage("{}")
	if len(e.Payload) > 0 {
		payload = json.RawMessage(e.Payload)
	}
	return json.Marshal(map[string]any{
		"id":               pgutil.UUIDToString(e.ID),
		"match_id":         pgutil.UUIDToString(e.MatchID),
		"organization_id":  pgutil.UUIDToString(e.OrganizationID),
		"sequence_number":  e.SequenceNumber,
		"event_type":       string(e.EventType),
		"team_id":          pgutil.UUIDToString(e.TeamID),
		"player_id":        pgutil.UUIDToString(e.PlayerID),
		"period":           e.Period,
		"clock_seconds":    e.ClockSeconds,
		"payload":          payload,
		"recorded_by":      pgutil.UUIDToString(e.RecordedBy),
		"recorded_at":      e.RecordedAt.Time.UTC().Format(time.RFC3339),
		"cancels_event_id": pgutil.UUIDToString(e.CancelsEventID),
		"created_at":       e.CreatedAt.Time.UTC().Format(time.RFC3339),
	})
}

func uuidEquals(a, b pgtype.UUID) bool {
	return a.Valid && b.Valid && a.Bytes == b.Bytes
}
