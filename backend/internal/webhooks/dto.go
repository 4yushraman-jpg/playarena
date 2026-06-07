package webhooks

// CreateRequest is the body for POST /api/v1/organizations/{slug}/webhooks.
type CreateRequest struct {
	URL         string  `json:"url"`
	Description *string `json:"description"`
}

// UpdateActiveRequest is the body for PATCH /api/v1/organizations/{slug}/webhooks/{id}/active.
type UpdateActiveRequest struct {
	Active bool `json:"active"`
}

// Response is the API representation of a webhook endpoint.
// secret_ciphertext is never exposed; raw_secret is set only on creation.
type Response struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	URL            string  `json:"url"`
	Description    *string `json:"description"`
	Active         bool    `json:"active"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// CreateResponse extends Response with the raw secret (shown once only).
type CreateResponse struct {
	Response
	RawSecret string `json:"secret"`
}

// ListResponse wraps a paginated list of webhook endpoints.
type ListResponse struct {
	Endpoints []Response `json:"endpoints"`
	Total     int        `json:"total"`
}
