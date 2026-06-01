package tournament_registrations

import "errors"

var (
	ErrRegistrationNotFound = errors.New("registration not found")
	ErrTournamentNotFound   = errors.New("tournament not found")
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrTeamNotFound         = errors.New("team not found")

	// ErrForbidden is the BOLA guard.
	ErrForbidden = errors.New("access denied: you do not have permission to modify this tournament's registrations")

	// Rule 2 — Tournament must be in registration_open status.
	ErrRegistrationClosed = errors.New("registrations are not open; tournament must be in registration_open status")

	// Rule 3 — Registration window.
	ErrWindowNotOpen = errors.New("registration window has not opened yet")
	ErrWindowClosed  = errors.New("registration window has closed")

	// Rule 4 — Duplicate registration.
	ErrAlreadyRegistered = errors.New("this team is already registered for the tournament")

	// Rule 5 — Team eligibility.
	ErrTeamNotActive = errors.New("only active teams may register for tournaments")
	// ErrCrossOrgRegistration is returned when the team does not belong to the
	// organization resolved from the URL slug (multi-tenant safety Rule 1).
	ErrCrossOrgRegistration = errors.New("team does not belong to this organization; cross-org registration is forbidden")

	// Rule 6 — Team must have at least one active member.
	ErrEmptyTeam = errors.New("team has no active members; at least one active member is required before registering")

	// Rule 7 — Capacity.
	ErrTournamentFull = errors.New("tournament has reached its maximum participant capacity")

	// Status lifecycle.
	ErrInvalidStatus           = errors.New("invalid registration status; valid values: pending, approved, rejected, withdrawn, disqualified")
	ErrInvalidStatusTransition = errors.New("invalid status transition")
)
