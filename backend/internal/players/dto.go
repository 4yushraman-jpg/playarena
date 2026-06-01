package players

// ── constants ─────────────────────────────────────────────────────────────────

const (
	// DefaultListLimit is the default page size for GET /players.
	DefaultListLimit = int32(50)
	// MaxListLimit is the largest page the API will honour.
	MaxListLimit = int32(200)
)

// ── request DTOs ──────────────────────────────────────────────────────────────

// CreateRequest is the payload for POST /api/v1/organizations/{slug}/players.
type CreateRequest struct {
	DisplayName  string  `json:"display_name"  validate:"required,min=1,max=255"`
	JerseyNumber *string `json:"jersey_number"`
	Position     *string `json:"position"`
	// height_cm: 51–299. Validated by the service layer.
	HeightCm *int16 `json:"height_cm"`
	// weight_kg: 21–299. Validated by the service layer.
	WeightKg     *int16  `json:"weight_kg"`
	DominantHand *string `json:"dominant_hand"` // left | right | ambidextrous
	Nationality  *string `json:"nationality"`   // 2-letter ISO 3166-1 alpha-2
	DateOfBirth  *string `json:"date_of_birth"` // YYYY-MM-DD
	Bio          *string `json:"bio"`
	// UserID links this profile to a platform user account. Optional.
	UserID *string `json:"user_id"`
}

// UpdateRequest is the payload for PATCH /api/v1/organizations/{slug}/players/{id}.
// All fields are optional; omitting or nulling a field leaves it unchanged.
type UpdateRequest struct {
	DisplayName  *string `json:"display_name"  validate:"omitempty,min=1,max=255"`
	JerseyNumber *string `json:"jersey_number"`
	Position     *string `json:"position"`
	HeightCm     *int16  `json:"height_cm"`
	WeightKg     *int16  `json:"weight_kg"`
	DominantHand *string `json:"dominant_hand"`
	Nationality  *string `json:"nationality"`
	DateOfBirth  *string `json:"date_of_birth"`
	Bio          *string `json:"bio"`
	// Status transitions: active | inactive | injured | suspended | retired.
	// Setting inactive is equivalent to a soft delete.
	Status *string `json:"status"`
}

// ListParams carries validated pagination and filter inputs.
type ListParams struct {
	Limit        int32
	Offset       int32
	StatusFilter *string // nil = all non-inactive statuses
	Search       *string // nil = no name filter
}

// ── response DTOs ─────────────────────────────────────────────────────────────

// Response is the unified player representation returned by all endpoints.
type Response struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	UserID         *string `json:"user_id,omitempty"`
	DisplayName    string  `json:"display_name"`
	JerseyNumber   *string `json:"jersey_number,omitempty"`
	Position       *string `json:"position,omitempty"`
	HeightCm       *int16  `json:"height_cm,omitempty"`
	WeightKg       *int16  `json:"weight_kg,omitempty"`
	DominantHand   *string `json:"dominant_hand,omitempty"`
	Nationality    *string `json:"nationality,omitempty"`
	DateOfBirth    *string `json:"date_of_birth,omitempty"` // YYYY-MM-DD
	// Status reflects the current player lifecycle state. "inactive" means the
	// player was soft-deleted via the DELETE endpoint. The record is kept
	// permanently; GET by ID returns it with this status so that historical
	// team membership and match event data remain resolvable.
	Status    string  `json:"status"`
	Bio       *string `json:"bio,omitempty"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// ListResponse wraps a page of players with pagination metadata.
type ListResponse struct {
	Players []Response `json:"players"`
	Total   int64      `json:"total"`
	Limit   int        `json:"limit"`
	Offset  int        `json:"offset"`
}
