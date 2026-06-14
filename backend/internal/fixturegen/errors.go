package fixturegen

import "errors"

var (
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrTournamentNotFound   = errors.New("tournament not found")

	// ErrForbidden is the BOLA guard: caller's org context does not match the
	// target organization.
	ErrForbidden = errors.New("access denied: you do not have permission to modify this organization's tournaments")

	// Generation state guards (Deliverable 7).
	// ErrNotRegistrationClosed: generation may only run from registration_closed.
	ErrNotRegistrationClosed = errors.New("fixtures can only be generated while the tournament is in registration_closed")
	// ErrFixturesExist: generation refuses to run when any match already exists
	// (prevents double / partial / re-generation corruption).
	ErrFixturesExist = errors.New("fixtures already exist for this tournament; generation cannot run again")
	// ErrRegistrationsChanged: the approved-registration set changed between
	// preview and confirm (e.g. a late registration); re-preview before confirming.
	ErrRegistrationsChanged = errors.New("the approved participants changed since preview; please review and try again")

	// Input / config errors.
	ErrInvalidTournamentID = errors.New("invalid tournament id")
	ErrInvalidFormat       = errors.New("unsupported format; supported: round_robin, league, knockout, group_knockout")
	ErrInvalidSeedStrategy = errors.New("seed strategy must be manual, random, or registration")
	ErrTooFewParticipants  = errors.New("at least 2 approved participants are required to generate fixtures")
	ErrInvalidGroupConfig  = errors.New("invalid group configuration for the participant count")

	// Resolve-qualifiers errors (group_knockout).
	ErrNotGroupKnockout     = errors.New("qualifier resolution applies only to group_knockout tournaments")
	ErrTournamentNotOngoing = errors.New("the tournament must be ongoing to resolve qualifiers")
	ErrGroupStageIncomplete = errors.New("the group stage is not complete; all group matches must be concluded first")
	ErrNoQualifierMatches   = errors.New("no unresolved knockout qualifier slots were found")
	ErrQualifierUnavailable = errors.New("a required group qualifier position has no participant")
)
