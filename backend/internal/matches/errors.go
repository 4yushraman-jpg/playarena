package matches

import "errors"

var (
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrTournamentNotFound   = errors.New("tournament not found")
	ErrMatchNotFound        = errors.New("match not found")
	ErrTeamNotFound         = errors.New("team not found")
	ErrPlayerNotFound       = errors.New("player not found")

	// ErrForbidden is the BOLA guard: caller's JWT org context does not match
	// the target organization.
	ErrForbidden = errors.New("access denied: you do not have permission to modify this organization's matches")

	// ErrTournamentNotOngoing is returned when a match create is attempted on a
	// tournament that is not in ongoing status.
	ErrTournamentNotOngoing = errors.New("tournament must be in ongoing status to create or progress matches")

	// Participant validation errors.
	ErrMixedParticipantTypes    = errors.New("match participants must match the tournament participant type: provide both team IDs for team tournaments or both player IDs for individual tournaments")
	ErrMissingParticipants      = errors.New("both home and away participants are required; provide home_team_id + away_team_id or home_player_id + away_player_id")
	ErrDuplicateParticipants    = errors.New("home and away participants must be different")
	ErrParticipantCrossOrg      = errors.New("participant does not belong to this organization")
	ErrParticipantNotRegistered = errors.New("participant has no approved registration for this tournament")

	// Winner validation errors.
	ErrWinnerNotAllowed     = errors.New("winner may only be set when match status is completed")
	ErrWinnerNotParticipant = errors.New("winner must be one of the match participants (home or away)")
	// ErrWinnerScoreMismatch is returned when the declared winner is inconsistent
	// with the computed final score: home wins require winner = home participant,
	// away wins require winner = away participant, and draws require no winner.
	// Walkovers are exempt because the score is always 0-0 for administrative wins.
	ErrWinnerScoreMismatch = errors.New("winner is inconsistent with the final score: home win requires home winner, away win requires away winner, equal score requires no winner")

	// Status lifecycle errors.
	ErrInvalidStatus           = errors.New("invalid match status; valid values: scheduled, live, completed, cancelled, abandoned")
	ErrInvalidStatusTransition = errors.New("invalid status transition")
	ErrMatchNotUpdatable       = errors.New("match is in a terminal status and cannot be updated")
	ErrMatchAlreadyCancelled   = errors.New("match is already cancelled")

	// Input parsing errors.
	ErrInvalidTimestamp    = errors.New("timestamps must be in RFC3339 format (e.g. 2025-01-15T10:00:00Z)")
	ErrInvalidTournamentID = errors.New("invalid tournament_id")
)
