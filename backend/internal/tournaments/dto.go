package tournaments

// ── constants ─────────────────────────────────────────────────────────────────

const (
	DefaultListLimit = int32(50)
	MaxListLimit     = int32(200)
	DefaultCurrency  = "INR"
)

// ── request DTOs ──────────────────────────────────────────────────────────────

// CreateRequest is the payload for POST /api/v1/organizations/{slug}/tournaments.
type CreateRequest struct {
	Name   string `json:"name"   validate:"required,min=2,max=255"`
	Sport  string `json:"sport"  validate:"required,min=2"`
	Format string `json:"format" validate:"required"`
	// ParticipantType defaults to "team" when omitted.
	ParticipantType *string `json:"participant_type"`
	Description     *string `json:"description"`
	BannerURL       *string `json:"banner_url"`
	// PrizePool is a decimal string (e.g. "10000.00"). Validated by service.
	PrizePool *string `json:"prize_pool"`
	// Currency is a 3-letter ISO 4217 code. Defaults to "INR".
	Currency        *string `json:"currency"`
	MaxParticipants *int16  `json:"max_participants"`
	MinParticipants *int16  `json:"min_participants"`
	// Timestamps in RFC3339 format. Validated: opens_at < closes_at <= starts_at <= ends_at.
	RegistrationOpensAt  *string `json:"registration_opens_at"`
	RegistrationClosesAt *string `json:"registration_closes_at"`
	StartsAt             *string `json:"starts_at"`
	EndsAt               *string `json:"ends_at"`
	Venue                *string `json:"venue"`
	City                 *string `json:"city"`
	// Country is a 2-letter ISO 3166-1 alpha-2 code.
	Country *string `json:"country"`
	Rules   *string `json:"rules"`
}

// UpdateRequest is the payload for PATCH /api/v1/organizations/{slug}/tournaments/{id}.
// All fields are optional. slug is immutable after creation.
// Use Status to advance the tournament lifecycle; invalid transitions are rejected.
type UpdateRequest struct {
	Name                 *string `json:"name"            validate:"omitempty,min=2,max=255"`
	Description          *string `json:"description"`
	Sport                *string `json:"sport"`
	Format               *string `json:"format"`
	ParticipantType      *string `json:"participant_type"`
	BannerURL            *string `json:"banner_url"`
	PrizePool            *string `json:"prize_pool"`
	Currency             *string `json:"currency"`
	MaxParticipants      *int16  `json:"max_participants"`
	MinParticipants      *int16  `json:"min_participants"`
	RegistrationOpensAt  *string `json:"registration_opens_at"`
	RegistrationClosesAt *string `json:"registration_closes_at"`
	StartsAt             *string `json:"starts_at"`
	EndsAt               *string `json:"ends_at"`
	Venue                *string `json:"venue"`
	City                 *string `json:"city"`
	Country              *string `json:"country"`
	Rules                *string `json:"rules"`
	// Status drives lifecycle transitions.
	// Allowed: draft→registration_open, registration_open→registration_closed,
	// registration_closed→ongoing, ongoing→completed, any→cancelled.
	Status *string `json:"status"`
}

// ListParams carries validated pagination and filter inputs.
type ListParams struct {
	Limit        int32
	Offset       int32
	StatusFilter *string
	Search       *string
}

// ── response DTO ──────────────────────────────────────────────────────────────

// Response is the unified tournament representation returned by all endpoints.
type Response struct {
	ID              string  `json:"id"`
	OrganizationID  string  `json:"organization_id"`
	Name            string  `json:"name"`
	Slug            string  `json:"slug"`
	Description     *string `json:"description,omitempty"`
	Sport           string  `json:"sport"`
	Format          string  `json:"format"`
	ParticipantType string  `json:"participant_type"`
	// Status reflects the current lifecycle state.
	// "cancelled" means the tournament was soft-deleted via DELETE.
	// GetTournamentByID returns cancelled tournaments so that future
	// registration and match history references remain resolvable.
	Status               string  `json:"status"`
	BannerURL            *string `json:"banner_url,omitempty"`
	PrizePool            *string `json:"prize_pool,omitempty"` // decimal string
	Currency             string  `json:"currency"`
	MaxParticipants      *int16  `json:"max_participants,omitempty"`
	MinParticipants      *int16  `json:"min_participants,omitempty"`
	RegistrationOpensAt  *string `json:"registration_opens_at,omitempty"`
	RegistrationClosesAt *string `json:"registration_closes_at,omitempty"`
	StartsAt             *string `json:"starts_at,omitempty"`
	EndsAt               *string `json:"ends_at,omitempty"`
	Venue                *string `json:"venue,omitempty"`
	City                 *string `json:"city,omitempty"`
	Country              *string `json:"country,omitempty"`
	Rules                *string `json:"rules,omitempty"`
	CreatedBy            *string `json:"created_by,omitempty"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

// ListResponse wraps a paginated list of tournaments.
type ListResponse struct {
	Tournaments []Response `json:"tournaments"`
	Total       int64      `json:"total"`
	Limit       int        `json:"limit"`
	Offset      int        `json:"offset"`
}

// ── standings response DTOs ───────────────────────────────────────────────────

// StandingsRowResponse is one participant's record in the standings table.
type StandingsRowResponse struct {
	Position        int    `json:"position"`
	ParticipantID   string `json:"participant_id"`
	Played          int    `json:"played"`
	Wins            int    `json:"wins"`
	Losses          int    `json:"losses"`
	Draws           int    `json:"draws"`
	Points          int    `json:"points"`
	ScoreFor        int    `json:"score_for"`
	ScoreAgainst    int    `json:"score_against"`
	ScoreDifference int    `json:"score_difference"`
}

// PointSystemResponse describes the point values in use for this tournament.
type PointSystemResponse struct {
	WinPoints       int `json:"win_points"`
	DrawPoints      int `json:"draw_points"`
	LossPoints      int `json:"loss_points"`
	CloseMargin     int `json:"close_margin,omitempty"`
	CloseLossPoints int `json:"close_loss_points,omitempty"`
}

// StandingsResponse is the response for
// GET /api/v1/organizations/{slug}/tournaments/{id}/standings.
type StandingsResponse struct {
	TournamentID   string                 `json:"tournament_id"`
	TournamentName string                 `json:"tournament_name"`
	Format         string                 `json:"format"`
	Status         string                 `json:"status"`
	PointSystem    PointSystemResponse    `json:"point_system"`
	Standings      []StandingsRowResponse `json:"standings"`
}
