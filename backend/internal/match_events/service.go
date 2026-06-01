package match_events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/scoring"
)

// Service implements match event use-cases.
type Service struct {
	repo *Repository
	log  *slog.Logger
}

// NewService constructs a Service.
func NewService(repo *Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// ── public methods ────────────────────────────────────────────────────────────

// Create records a new match event.
//
// Service-layer responsibilities:
//   - BOLA guard
//   - event_type parsing and validation
//   - payload JSON validation
//   - optional participant UUID parsing
//   - recorded_at parsing (defaults to NOW() if absent)
//   - cancels_event_id consistency check (required for score_correction, forbidden otherwise)
//
// Repository-layer responsibilities (inside FOR UPDATE transaction):
//   - match status == live
//   - participant ownership
//   - lifecycle uniqueness
//   - score_correction target validation
//   - sequence_number computation
//   - audit log
func (s *Service) Create(
	ctx context.Context,
	orgSlug, matchID string,
	req CreateRequest,
	actorID, actorOrgID string,
) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	mid, err := pgutil.ParseUUID(matchID)
	if err != nil {
		return nil, ErrMatchNotFound
	}

	// Confirm match exists and belongs to this org before opening the transaction.
	match, err := s.repo.GetMatchByID(ctx, mid, org.ID)
	if err != nil {
		return nil, err
	}

	eventType, err := parseEventType(req.EventType)
	if err != nil {
		return nil, err
	}

	payload, err := parsePayload(req.Payload)
	if err != nil {
		return nil, err
	}

	// Validate that scoring event payloads carry the required fields.
	// This runs at write time so malformed events never enter the immutable log.
	if err := scoring.ValidateScoreEventPayload(eventType, payload); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidScorePayload, err.Error())
	}

	// For all_out, additionally verify that payload.team_id is one of the match
	// participants.  ValidateScoreEventPayload confirmed team_id is non-empty;
	// this guard rejects any value that does not belong to this specific match.
	if eventType == db.MatchEventTypeAllOut {
		homeID := pgutil.UUIDToString(match.HomeTeamID)
		awayID := pgutil.UUIDToString(match.AwayTeamID)
		if err := scoring.ValidateAllOutParticipant(payload, homeID, awayID); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidScorePayload, err.Error())
		}
	}

	teamUID := pgutil.ParseOptionalUUID(derefStr(req.TeamID))
	playerUID := pgutil.ParseOptionalUUID(derefStr(req.PlayerID))

	cancelsUID := pgutil.ParseOptionalUUID(derefStr(req.CancelsEventID))

	// cancels_event_id is required for score_correction and forbidden for all others.
	if eventType == db.MatchEventTypeScoreCorrection && !cancelsUID.Valid {
		return nil, ErrCancelsEventRequired
	}
	if eventType != db.MatchEventTypeScoreCorrection && cancelsUID.Valid {
		return nil, ErrCancelsEventNotAllowed
	}

	recordedAt, err := parseRecordedAt(req.RecordedAt)
	if err != nil {
		return nil, err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	event, err := s.repo.CreateWithAudit(ctx, createEventTxParams{
		matchID:        mid,
		organizationID: org.ID,
		eventType:      eventType,
		teamID:         teamUID,
		playerID:       playerUID,
		period:         req.Period,
		clockSeconds:   req.ClockSeconds,
		payload:        payload,
		recordedBy:     actorUID,
		recordedAt:     recordedAt,
		cancelsEventID: cancelsUID,
		actorID:        actorUID,
	})
	if err != nil {
		return nil, err
	}
	return eventToResponse(event), nil
}

// List returns a paginated event timeline for a match.
// No ownership check: any authenticated user may read the event timeline.
func (s *Service) List(ctx context.Context, orgSlug, matchID string, params ListParams) (*ListResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	mid, err := pgutil.ParseUUID(matchID)
	if err != nil {
		return nil, ErrMatchNotFound
	}

	// Confirm match exists and belongs to this org; return 404 if not found
	// rather than an empty list (which would be misleading).
	if _, err := s.repo.GetMatchByID(ctx, mid, org.ID); err != nil {
		return nil, err
	}

	if params.Limit <= 0 || params.Limit > MaxListLimit {
		params.Limit = DefaultListLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	events, err := s.repo.List(ctx, mid, org.ID, params)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.Count(ctx, mid, org.ID, params)
	if err != nil {
		return nil, err
	}

	resp := make([]Response, len(events))
	for i := range events {
		resp[i] = *eventToResponse(&events[i])
	}
	return &ListResponse{
		Events:        resp,
		Total:         total,
		Limit:         int(params.Limit),
		Offset:        int(params.Offset),
		EffectiveOnly: params.EffectiveOnly,
	}, nil
}

// GetByID retrieves a single match event.
// No ownership check: any authenticated user may read event details.
func (s *Service) GetByID(ctx context.Context, orgSlug, matchID, eventID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	mid, err := pgutil.ParseUUID(matchID)
	if err != nil {
		return nil, ErrMatchNotFound
	}

	if _, err := s.repo.GetMatchByID(ctx, mid, org.ID); err != nil {
		return nil, err
	}

	eid, err := pgutil.ParseUUID(eventID)
	if err != nil {
		return nil, ErrEventNotFound
	}

	event, err := s.repo.GetByID(ctx, eid, mid, org.ID)
	if err != nil {
		return nil, err
	}
	return eventToResponse(event), nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func assertOrgOwnership(actorOrgID, targetOrgID string) error {
	if actorOrgID == "" {
		return nil // platform admin
	}
	if actorOrgID != targetOrgID {
		return ErrForbidden
	}
	return nil
}

// parseEventType validates the event_type string against the schema-defined
// 21-value match_event_type ENUM. Returns ErrInvalidEventType for any unknown value.
func parseEventType(s string) (db.MatchEventType, error) {
	et := db.MatchEventType(strings.ToLower(strings.TrimSpace(s)))
	switch et {
	case db.MatchEventTypeMatchStarted, db.MatchEventTypeMatchEnded,
		db.MatchEventTypeHalfStarted, db.MatchEventTypeHalfEnded,
		db.MatchEventTypeTimeoutCalled, db.MatchEventTypeTimeoutEnded,
		db.MatchEventTypeRaidAttempt, db.MatchEventTypeRaidSuccessful,
		db.MatchEventTypeRaidEmpty, db.MatchEventTypeBonusPointAwarded,
		db.MatchEventTypeTackleSuccessful, db.MatchEventTypeSuperTackle,
		db.MatchEventTypeSuperRaid, db.MatchEventTypeDoOrDieRaid,
		db.MatchEventTypeAllOut,
		db.MatchEventTypePlayerOut, db.MatchEventTypePlayerRevived,
		db.MatchEventTypePlayerSubstituted, db.MatchEventTypePlayerInjured,
		db.MatchEventTypePenaltyAwarded, db.MatchEventTypeScoreCorrection:
		return et, nil
	}
	return "", ErrInvalidEventType
}

// parsePayload validates the request payload and returns a canonical []byte.
// nil or empty input is normalised to "{}".
// Non-object JSON (array, scalar) is rejected.
func parsePayload(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return []byte("{}"), nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, ErrInvalidPayload
	}
	return raw, nil
}

// parseRecordedAt parses an optional RFC3339 recorded_at string.
// A nil pointer defaults to time.Now().UTC().
func parseRecordedAt(s *string) (pgtype.Timestamptz, error) {
	if s == nil || *s == "" {
		return pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return pgtype.Timestamptz{}, ErrInvalidTimestamp
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}, nil
}

// eventToResponse converts a db.MatchEvent to the API response struct.
func eventToResponse(e *db.MatchEvent) *Response {
	payload := json.RawMessage("{}")
	if len(e.Payload) > 0 {
		payload = json.RawMessage(e.Payload)
	}
	return &Response{
		ID:             pgutil.UUIDToString(e.ID),
		MatchID:        pgutil.UUIDToString(e.MatchID),
		OrganizationID: pgutil.UUIDToString(e.OrganizationID),
		SequenceNumber: e.SequenceNumber,
		EventType:      string(e.EventType),
		TeamID:         uuidStringPtr(e.TeamID),
		PlayerID:       uuidStringPtr(e.PlayerID),
		Period:         e.Period,
		ClockSeconds:   e.ClockSeconds,
		Payload:        payload,
		RecordedBy:     uuidStringPtr(e.RecordedBy),
		RecordedAt:     e.RecordedAt.Time.UTC().Format(time.RFC3339),
		CancelsEventID: uuidStringPtr(e.CancelsEventID),
		CreatedAt:      e.CreatedAt.Time.UTC().Format(time.RFC3339),
	}
}

func uuidStringPtr(uid pgtype.UUID) *string {
	if !uid.Valid {
		return nil
	}
	s := pgutil.UUIDToString(uid)
	return &s
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
