package tournaments

import "errors"

var (
	ErrTournamentNotFound   = errors.New("tournament not found")
	ErrOrganizationNotFound = errors.New("organization not found")

	// ErrForbidden is the BOLA guard: caller's JWT org context does not match
	// the target organization.
	ErrForbidden = errors.New("access denied: you do not have permission to modify this organization's tournaments")

	ErrSlugAlreadyTaken     = errors.New("a tournament with this slug already exists in the organization")
	ErrSlugGenerationFailed = errors.New("could not generate a unique slug for this tournament name — try a more distinctive name")

	ErrInvalidFormat          = errors.New("invalid format; valid values: league, knockout, group_knockout, round_robin, double_elimination")
	ErrInvalidParticipantType = errors.New("invalid participant_type; valid values: team, individual")
	ErrInvalidStatus          = errors.New("invalid status; valid values: draft, registration_open, registration_closed, ongoing, completed, cancelled")
	ErrInvalidCurrency        = errors.New("currency must be a 3-letter ISO 4217 code (e.g. INR, USD)")
	ErrInvalidCountry         = errors.New("country must be a 2-letter ISO 3166-1 alpha-2 code (e.g. IN, US)")
	ErrInvalidPrizePool       = errors.New("prize_pool must be a valid non-negative decimal number (e.g. \"10000.00\")")
	ErrInvalidDateRange       = errors.New("invalid date range: registration_opens_at < registration_closes_at <= starts_at <= ends_at")
	ErrInvalidTimestamp       = errors.New("timestamps must be in RFC3339 format (e.g. 2025-01-15T10:00:00Z)")

	// ErrInvalidStatusTransition is returned when the requested status change
	// violates the allowed lifecycle order.
	// Allowed: draft→registration_open, registration_open→registration_closed,
	// registration_closed→ongoing, ongoing→completed, any→cancelled.
	ErrInvalidStatusTransition = errors.New("invalid status transition")
)
