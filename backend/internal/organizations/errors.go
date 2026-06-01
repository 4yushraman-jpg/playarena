package organizations

import "errors"

var (
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrSlugAlreadyTaken     = errors.New("organization slug is already taken")
	ErrSlugGenerationFailed = errors.New("could not generate a unique slug for this organization name — try providing a more distinctive name")
	ErrInvalidOrgType       = errors.New("invalid organization type; valid values: club, federation, school, corporate, independent")
	ErrInvalidCountryCode   = errors.New("country must be a 2-letter ISO 3166-1 alpha-2 code (e.g. IN, US)")

	// ErrForbidden is returned when an authenticated user attempts to modify an
	// organization they do not own. The BOLA / IDOR fix (C1) compares the target
	// organization's ID against the actor's JWT org context before proceeding.
	ErrForbidden = errors.New("access denied: you do not have permission to modify this organization")
)
