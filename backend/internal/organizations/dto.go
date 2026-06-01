package organizations

// ---- constants --------------------------------------------------------------

const (
	// DefaultListLimit is the page size used when no limit is supplied.
	DefaultListLimit = int32(50)
	// MaxListLimit is the largest page size the API will honour.
	MaxListLimit = int32(200)
)

// ---- request DTOs -----------------------------------------------------------

// CreateRequest is the payload for POST /api/v1/organizations.
type CreateRequest struct {
	Name        string  `json:"name"        validate:"required,min=2,max=255"`
	Type        string  `json:"type"        validate:"required"`
	Description *string `json:"description"`
	Website     *string `json:"website"`
	Email       *string `json:"email"       validate:"omitempty,email"`
	Phone       *string `json:"phone"`
	Country     *string `json:"country"`
	City        *string `json:"city"`
}

// UpdateRequest is the payload for PATCH /api/v1/organizations/{slug}.
// All fields are optional; omitting or null-ing a field leaves it unchanged.
type UpdateRequest struct {
	Name        *string `json:"name"        validate:"omitempty,min=2,max=255"`
	Type        *string `json:"type"`
	Description *string `json:"description"`
	Website     *string `json:"website"`
	Email       *string `json:"email"       validate:"omitempty,email"`
	Phone       *string `json:"phone"`
	Country     *string `json:"country"`
	City        *string `json:"city"`
}

// ListParams carries validated pagination inputs for GET /api/v1/organizations.
type ListParams struct {
	Limit  int32
	Offset int32
}

// ---- response DTOs ----------------------------------------------------------

// Response is the unified organization representation returned by all endpoints.
type Response struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Type        string  `json:"type"`
	Status      string  `json:"status"`
	Description *string `json:"description,omitempty"`
	Website     *string `json:"website,omitempty"`
	Email       *string `json:"email,omitempty"`
	Phone       *string `json:"phone,omitempty"`
	Country     *string `json:"country,omitempty"`
	City        *string `json:"city,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// ListResponse wraps a page of organizations with pagination metadata.
type ListResponse struct {
	Organizations []Response `json:"organizations"`
	Total         int        `json:"total"`
	Limit         int        `json:"limit"`
	Offset        int        `json:"offset"`
}
