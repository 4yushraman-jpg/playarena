package players

import "errors"

var (
	ErrPlayerNotFound       = errors.New("player not found")
	ErrOrganizationNotFound = errors.New("organization not found")

	// ErrForbidden is returned when the caller's org context does not match the
	// target organization (BOLA / IDOR prevention, mirrors organizations.ErrForbidden).
	ErrForbidden = errors.New("access denied: you do not have permission to modify this organization's players")

	// GP-1 self-profile errors
	ErrProfileExists     = errors.New("a player profile already exists for this user")
	ErrInvalidVisibility = errors.New("visibility must be one of: public, unlisted, private")
	ErrImmutableField    = errors.New("field cannot be modified via this endpoint")

	ErrInvalidStatus       = errors.New("invalid player status; valid values: active, inactive, injured, suspended, retired")
	ErrInvalidDominantHand = errors.New("dominant_hand must be one of: left, right, ambidextrous")
	ErrInvalidNationality  = errors.New("nationality must be a 2-letter ISO 3166-1 alpha-2 code (e.g. IN, US)")
	ErrInvalidDateOfBirth  = errors.New("date_of_birth must be in YYYY-MM-DD format and must be a date in the past")
)
