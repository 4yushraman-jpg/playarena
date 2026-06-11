package teams

// ── constants ─────────────────────────────────────────────────────────────────

const (
	DefaultListLimit = int32(50)
	MaxListLimit     = int32(200)
)

// ── team request DTOs ─────────────────────────────────────────────────────────

// CreateRequest is the payload for POST /api/v1/organizations/{slug}/teams.
type CreateRequest struct {
	Name        string  `json:"name"        validate:"required,min=1,max=255"`
	ShortName   *string `json:"short_name"` // 2–10 chars; validated by service
	Description *string `json:"description"`
	LogoURL     *string `json:"logo_url"`
	HomeCity    *string `json:"home_city"`
	HomeVenue   *string `json:"home_venue"`
	// founded_year: 1800–2100. Validated by the service layer.
	FoundedYear    *int16  `json:"founded_year"`
	PrimaryColor   *string `json:"primary_color"`   // #RRGGBB hex
	SecondaryColor *string `json:"secondary_color"` // #RRGGBB hex
}

// UpdateRequest is the payload for PATCH /api/v1/organizations/{slug}/teams/{id}.
// All fields are optional; omitting or nulling a field leaves it unchanged.
// slug is immutable after creation.
type UpdateRequest struct {
	Name           *string `json:"name"        validate:"omitempty,min=1,max=255"`
	ShortName      *string `json:"short_name"`
	Description    *string `json:"description"`
	LogoURL        *string `json:"logo_url"`
	HomeCity       *string `json:"home_city"`
	HomeVenue      *string `json:"home_venue"`
	FoundedYear    *int16  `json:"founded_year"`
	PrimaryColor   *string `json:"primary_color"`
	SecondaryColor *string `json:"secondary_color"`
	// Status transitions: active | inactive | disbanded.
	// Setting disbanded via PATCH is equivalent to calling DELETE.
	Status *string `json:"status"`
}

// ListParams carries validated pagination and filter inputs.
type ListParams struct {
	Limit        int32
	Offset       int32
	StatusFilter *string
	Search       *string
}

// ── team response DTOs ────────────────────────────────────────────────────────

// Response is the unified team representation returned by all team endpoints.
type Response struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	Name           string  `json:"name"`
	ShortName      *string `json:"short_name,omitempty"`
	Slug           string  `json:"slug"`
	Description    *string `json:"description,omitempty"`
	LogoURL        *string `json:"logo_url,omitempty"`
	HomeCity       *string `json:"home_city,omitempty"`
	HomeVenue      *string `json:"home_venue,omitempty"`
	FoundedYear    *int16  `json:"founded_year,omitempty"`
	PrimaryColor   *string `json:"primary_color,omitempty"`
	SecondaryColor *string `json:"secondary_color,omitempty"`
	// Status reflects the current team lifecycle state.
	// "disbanded" means the team was soft-deleted via the DELETE endpoint.
	// GetTeamByID returns disbanded teams so that match history and tournament
	// brackets continue to resolve to a valid record.
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ListResponse wraps a paginated list of teams.
type ListResponse struct {
	Teams  []Response `json:"teams"`
	Total  int64      `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

// ── membership request DTOs ───────────────────────────────────────────────────

// AddMemberRequest is the payload for
// POST /api/v1/organizations/{slug}/teams/{id}/members.
type AddMemberRequest struct {
	// PlayerID must be a player that belongs to the same organization.
	PlayerID     string  `json:"player_id"    validate:"required,uuid"`
	Role         *string `json:"role"`          // defaults to "player"
	JerseyNumber *string `json:"jersey_number"` // overrides players.jersey_number for this team
	Notes        *string `json:"notes"`
}

// ── membership response DTOs ──────────────────────────────────────────────────

// MembershipResponse is returned by the membership endpoints.
type MembershipResponse struct {
	ID             string  `json:"id"`
	TeamID         string  `json:"team_id"`
	PlayerID       string  `json:"player_id"`
	OrganizationID string  `json:"organization_id"`
	Role           string  `json:"role"`
	JerseyNumber   *string `json:"jersey_number,omitempty"`
	Status         string  `json:"status"`
	JoinedAt       string  `json:"joined_at"`
	LeftAt         *string `json:"left_at,omitempty"`
	Notes          *string `json:"notes,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	// PlayerDisplayName is the player's display_name at query time, embedded to
	// avoid N+1 lookups on the roster UI.
	PlayerDisplayName string `json:"player_display_name"`
}

// MemberListResponse wraps a list of active memberships.
type MemberListResponse struct {
	Members []MembershipResponse `json:"members"`
}
