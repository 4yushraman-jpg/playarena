package tournament_registrations

// ── constants ─────────────────────────────────────────────────────────────────

const (
	DefaultListLimit = int32(50)
	MaxListLimit     = int32(200)
)

// ── request DTOs ──────────────────────────────────────────────────────────────

// CreateRequest is the payload for
// POST /api/v1/organizations/{slug}/tournaments/{tournamentId}/registrations.
//
// Phase 7B supports team-based registrations only. team_id is required.
type CreateRequest struct {
	// TeamID must be a team that belongs to the same organization as the tournament.
	TeamID string  `json:"team_id" validate:"required,uuid"`
	Notes  *string `json:"notes"`
}

// UpdateRequest is the payload for
// PATCH /api/v1/organizations/{slug}/tournaments/{tournamentId}/registrations/{registrationId}.
// All fields are optional; omitting leaves the current value unchanged.
//
// Status transitions:
//
//	pending → approved | rejected
//	approved → withdrawn | disqualified
//	pending → withdrawn
//
// Terminal states (no further transitions): rejected, withdrawn, disqualified.
type UpdateRequest struct {
	Status     *string `json:"status"`      // see allowed transitions above
	SeedNumber *int16  `json:"seed_number"` // assigned by organizer after reg closes
	Notes      *string `json:"notes"`
}

// ListParams carries validated pagination and filter inputs.
type ListParams struct {
	Limit        int32
	Offset       int32
	StatusFilter *string
}

// ── response DTOs ─────────────────────────────────────────────────────────────

// Response is the unified registration representation returned by all endpoints.
type Response struct {
	ID           string `json:"id"`
	TournamentID string `json:"tournament_id"`
	// OrganizationID is the registrant's organization — same as the URL org for Phase 7B.
	OrganizationID string  `json:"organization_id"`
	TeamID         *string `json:"team_id,omitempty"`
	PlayerID       *string `json:"player_id,omitempty"`
	SeedNumber     *int16  `json:"seed_number,omitempty"`
	Status         string  `json:"status"`
	RegisteredBy   *string `json:"registered_by,omitempty"`
	RegisteredAt   string  `json:"registered_at"`
	ApprovedBy     *string `json:"approved_by,omitempty"`
	ApprovedAt     *string `json:"approved_at,omitempty"`
	Notes          *string `json:"notes,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// ListResponse wraps a paginated list of registrations.
type ListResponse struct {
	Registrations []Response `json:"registrations"`
	Total         int64      `json:"total"`
	Limit         int        `json:"limit"`
	Offset        int        `json:"offset"`
}
