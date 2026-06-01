package match_events

import "errors"

var (
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrMatchNotFound        = errors.New("match not found")
	ErrEventNotFound        = errors.New("match event not found")

	// ErrForbidden is the BOLA guard.
	ErrForbidden = errors.New("access denied: you do not have permission to record events for this organization's matches")

	// ErrMatchNotLive is returned when an event is submitted for a non-live match.
	ErrMatchNotLive = errors.New("events may only be recorded when the match is live")

	// Input parsing errors.
	ErrInvalidEventType = errors.New("invalid event type; must be one of the schema-defined match_event_type values")
	ErrInvalidPayload   = errors.New("payload must be a valid JSON object (e.g. {} or {\"key\": \"value\"})")
	ErrInvalidTimestamp = errors.New("timestamps must be in RFC3339 format (e.g. 2025-01-15T10:00:00Z)")

	// Participant validation errors.
	ErrTeamNotParticipant   = errors.New("team_id must be the home or away team of this match")
	ErrPlayerNotParticipant = errors.New("player_id must be a participant in this match")
	ErrPlayerNotOnTeam      = errors.New("player_id does not belong to the supplied team_id")

	// Lifecycle uniqueness errors.
	ErrDuplicateLifecycleEvent = errors.New("a match_started or match_ended event already exists for this match")

	// Score correction errors.
	ErrCancelsEventRequired   = errors.New("cancels_event_id is required for score_correction events")
	ErrCancelsEventNotAllowed = errors.New("cancels_event_id is only valid for score_correction events")
	ErrCancelsEventNotFound   = errors.New("cancels_event_id references an event that does not exist")
	ErrCancelsEventCrossMatch = errors.New("cancels_event_id must reference an event in the same match")
	ErrEventAlreadyCancelled  = errors.New("the target event has already been cancelled by a score_correction")
	ErrCannotCancelCorrection = errors.New("a score_correction event cannot cancel another score_correction")
)
