package match_events

import "encoding/json"

// ── request DTOs ──────────────────────────────────────────────────────────────

// CreateRequest is the payload for
// POST /api/v1/organizations/{slug}/matches/{matchId}/events.
//
// event_type is required and must be one of the 21 schema-defined values.
// sequence_number is never accepted from the client — always computed server-side.
// recorded_by is always derived from the authenticated JWT user.
//
// Correction rules (event_type = score_correction):
//   - cancels_event_id is required.
//   - Target event must exist in the same match.
//   - Target event must not already be cancelled.
//   - Target event must not itself be a score_correction.
type CreateRequest struct {
	EventType      string          `json:"event_type" validate:"required"`
	TeamID         *string         `json:"team_id"`
	PlayerID       *string         `json:"player_id"`
	Period         *int16          `json:"period"`
	ClockSeconds   *int32          `json:"clock_seconds"`
	Payload        json.RawMessage `json:"payload"`          // must be JSON object; defaults to {}
	RecordedAt     *string         `json:"recorded_at"`      // RFC3339; defaults to server NOW()
	CancelsEventID *string         `json:"cancels_event_id"` // required only for score_correction
}

// ── response DTOs ─────────────────────────────────────────────────────────────

// Response is the unified match event representation returned by all endpoints.
type Response struct {
	ID             string          `json:"id"`
	MatchID        string          `json:"match_id"`
	OrganizationID string          `json:"organization_id"`
	SequenceNumber int64           `json:"sequence_number"`
	EventType      string          `json:"event_type"`
	TeamID         *string         `json:"team_id,omitempty"`
	PlayerID       *string         `json:"player_id,omitempty"`
	Period         *int16          `json:"period,omitempty"`
	ClockSeconds   *int32          `json:"clock_seconds,omitempty"`
	Payload        json.RawMessage `json:"payload"`
	RecordedBy     *string         `json:"recorded_by,omitempty"`
	RecordedAt     string          `json:"recorded_at"`
	CancelsEventID *string         `json:"cancels_event_id,omitempty"`
	CreatedAt      string          `json:"created_at"`
}

// ListResponse wraps a paginated event listing.
type ListResponse struct {
	Events        []Response `json:"events"`
	Total         int64      `json:"total"`
	Limit         int        `json:"limit"`
	Offset        int        `json:"offset"`
	EffectiveOnly bool       `json:"effective_only"`
}
