package scoring

import (
	"encoding/json"
	"errors"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

var (
	// ErrPayloadMissingPoints is returned when a raid_successful or penalty_awarded
	// event is submitted without a positive payload.points field.
	ErrPayloadMissingPoints = errors.New("scoring event payload must include \"points\" > 0")

	// ErrPayloadMissingTeamID is returned when an all_out event is submitted
	// without a non-empty payload.team_id identifying the eliminated team.
	ErrPayloadMissingTeamID = errors.New("all_out payload must include a non-empty \"team_id\"")

	// ErrPayloadMissingBonusPoints is returned when an all_out event is submitted
	// without a positive payload.bonus_points field.
	ErrPayloadMissingBonusPoints = errors.New("all_out payload must include \"bonus_points\" > 0")

	// ErrPayloadTeamIDNotParticipant is returned when an all_out event is submitted
	// with a team_id that does not match either match participant.
	ErrPayloadTeamIDNotParticipant = errors.New("all_out payload team_id must be the UUID of a match participant (home or away team)")
)

// ValidateScoreEventPayload validates that scoring event payloads contain the
// fields required for correct score derivation.  It is called at write time so
// that malformed events are rejected before they enter the immutable log.
// Returns nil for non-scoring event types and for types with no payload
// requirements (bonus_point_awarded, tackle_successful, super_tackle).
func ValidateScoreEventPayload(et db.MatchEventType, payload []byte) error {
	switch et {
	case db.MatchEventTypeRaidSuccessful, db.MatchEventTypePenaltyAwarded:
		return validatePoints(normalizePayload(payload))
	case db.MatchEventTypeAllOut:
		return validateAllOut(normalizePayload(payload))
	}
	return nil
}

// validatePoints checks that payload contains a positive integer "points" field.
func validatePoints(payload []byte) error {
	var obj struct {
		Points *int `json:"points"`
	}
	if err := json.Unmarshal(payload, &obj); err != nil {
		return ErrPayloadMissingPoints
	}
	if obj.Points == nil || *obj.Points <= 0 {
		return ErrPayloadMissingPoints
	}
	return nil
}

// validateAllOut checks that an all_out payload contains team_id and bonus_points.
func validateAllOut(payload []byte) error {
	var obj struct {
		TeamID      *string `json:"team_id"`
		BonusPoints *int    `json:"bonus_points"`
	}
	if err := json.Unmarshal(payload, &obj); err != nil {
		return ErrPayloadMissingTeamID
	}
	if obj.TeamID == nil || *obj.TeamID == "" {
		return ErrPayloadMissingTeamID
	}
	if obj.BonusPoints == nil || *obj.BonusPoints <= 0 {
		return ErrPayloadMissingBonusPoints
	}
	return nil
}

// normalizePayload returns "{}" for nil/empty payloads so json.Unmarshal
// always operates on a valid JSON object.
func normalizePayload(p []byte) []byte {
	if len(p) == 0 {
		return []byte("{}")
	}
	return p
}

// ValidateAllOutParticipant validates that the all_out payload's team_id matches
// one of the match participants.  homeTeamID and awayTeamID are the UUID strings
// returned by pgutil.UUIDToString for the match's HomeTeamID and AwayTeamID
// columns; an empty string means the slot is unset (individual-format match).
//
// This is called after ValidateScoreEventPayload has confirmed team_id is
// non-empty and bonus_points > 0.  A garbage string, a UUID from another match,
// or an absent team slot all return ErrPayloadTeamIDNotParticipant.
func ValidateAllOutParticipant(payload []byte, homeTeamID, awayTeamID string) error {
	var obj struct {
		TeamID *string `json:"team_id"`
	}
	if err := json.Unmarshal(normalizePayload(payload), &obj); err != nil || obj.TeamID == nil || *obj.TeamID == "" {
		return ErrPayloadMissingTeamID
	}
	id := *obj.TeamID
	// Both empty means individual-format match — no teams exist to eliminate.
	if homeTeamID == "" && awayTeamID == "" {
		return ErrPayloadTeamIDNotParticipant
	}
	if id != homeTeamID && id != awayTeamID {
		return ErrPayloadTeamIDNotParticipant
	}
	return nil
}
