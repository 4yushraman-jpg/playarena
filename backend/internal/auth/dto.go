package auth

// LoginRequest is the payload for POST /api/v1/auth/login.
//
// Multi-org behaviour:
//   - If the user belongs to exactly one organization, organization_id is
//     optional; the service selects it automatically.
//   - If the user belongs to multiple organizations, organization_id is
//     required. Omitting it returns ErrOrganizationRequired with the org list.
//   - Platform admins (super-admins with platform-scoped roles) omit
//     organization_id to receive a platform-level access token.
type LoginRequest struct {
	Email          string `json:"email"           validate:"required,email"`
	Password       string `json:"password"        validate:"required,min=8"`
	OrganizationID string `json:"organization_id" validate:"omitempty,uuid"`
}

// LoginResponse is returned on a successful authentication.
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// RefreshRequest is the payload for POST /api/v1/auth/refresh.
//
// The refresh token is org-agnostic. organization_id selects which org context
// the new access token should carry. Follows the same selection rules as
// LoginRequest: optional for single-org users and platform admins.
type RefreshRequest struct {
	RefreshToken   string `json:"refresh_token"   validate:"required"`
	OrganizationID string `json:"organization_id" validate:"omitempty,uuid"`
}

// RefreshResponse is returned on a successful token refresh.
// A new refresh_token is always issued (token rotation); the client must
// replace the stored token with the new value immediately.
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// LogoutRequest is the payload for POST /api/v1/auth/logout.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// MeResponse is the payload returned by GET /api/v1/auth/me (service layer).
// The HTTP handler augments this with role and organization_id from the token.
type MeResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	FullName string `json:"full_name"`
	Status   string `json:"status"`
}
