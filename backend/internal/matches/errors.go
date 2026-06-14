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

	// Walkover errors.
	// ErrInvalidWalkoverWinner is returned when the winner field is not
	// exactly "home" or "away".
	ErrInvalidWalkoverWinner = errors.New("walkover winner must be \"home\" or \"away\"")
	// ErrWalkoverReasonRequired is returned when no reason is supplied.
	ErrWalkoverReasonRequired = errors.New("a reason is required to award a walkover")
	// ErrWalkoverNeedsParticipants is returned when the match has an unresolved
	// (TBD/NULL) participant slot — a walkover cannot be awarded into a bracket
	// slot whose opponent is not yet known.
	ErrWalkoverNeedsParticipants = errors.New("both participants must be assigned before a walkover can be awarded")

	// Bracket-progression errors (FE-8B).
	// ErrMatchHasTBDSlot is the I1 guard: a match with an unresolved (TBD/NULL)
	// participant slot cannot be started, completed, or walked over.
	ErrMatchHasTBDSlot = errors.New("match has an unassigned participant slot and cannot be started or concluded")
	// ErrNextMatchLinkIncomplete is returned when next_match_id and
	// next_match_slot are not both set or both absent.
	ErrNextMatchLinkIncomplete = errors.New("next_match_id and next_match_slot must be provided together")
	// ErrInvalidNextSlot is returned when next_match_slot is not 1 (home) or 2 (away).
	ErrInvalidNextSlot = errors.New("next_match_slot must be 1 (home) or 2 (away)")
	// ErrNextMatchNotFound is returned when the linked successor match does not
	// exist within the organization.
	ErrNextMatchNotFound = errors.New("next match (bracket successor) not found")
	// ErrNextMatchCrossTournament is returned when the linked successor belongs
	// to a different tournament (I5 bracket-integrity guard).
	ErrNextMatchCrossTournament = errors.New("next match must belong to the same tournament")
	// ErrSelfLink is returned when a match is linked to advance into itself.
	ErrSelfLink = errors.New("a match cannot advance into itself")
	// ErrDownstreamLocked is the I3 guard: a winner cannot be propagated into a
	// successor that is no longer scheduled (already live or concluded). The
	// completion/walkover that triggered the propagation is rolled back.
	ErrDownstreamLocked = errors.New("the next match has already started or concluded; resolve the bracket manually before completing this match")
	// ErrBracketInconsistent is an internal integrity violation: a linked
	// successor was found in a different tournament than its feeder.
	ErrBracketInconsistent = errors.New("bracket linkage is inconsistent")
	// ErrSlotAlreadyFed is returned when another match already advances into the
	// requested (successor, slot) — two feeders cannot share one slot, as the
	// second would overwrite the first's propagated winner.
	ErrSlotAlreadyFed = errors.New("another match already advances into that next-match slot")

	// Input parsing errors.
	ErrInvalidTimestamp    = errors.New("timestamps must be in RFC3339 format (e.g. 2025-01-15T10:00:00Z)")
	ErrInvalidTournamentID = errors.New("invalid tournament_id")
)
