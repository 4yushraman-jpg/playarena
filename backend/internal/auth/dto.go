package auth

// ---- login ------------------------------------------------------------------

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

// ---- refresh ----------------------------------------------------------------

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

// ---- logout -----------------------------------------------------------------

// LogoutRequest is the payload for POST /api/v1/auth/logout.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// ---- register ---------------------------------------------------------------

// RegisterRequest is the payload for POST /api/v1/auth/register.
//
// full_name is split by the service into first_name / last_name for storage.
// username must be 3–30 characters and contain only letters, digits, and
// underscores (mirrors the DB CHECK constraint on users.username).
type RegisterRequest struct {
	Email    string `json:"email"     validate:"required,email"`
	Username string `json:"username"  validate:"required,min=3,max=30,alphanum_under"`
	FullName string `json:"full_name" validate:"required,min=1"`
	// max=72 matches maxPasswordBytes in passwords.go. The validator enforces
	// this by rune count; HashPassword enforces it by byte count. Together they
	// prevent bcrypt's 72-byte silent truncation from weakening any password.
	Password string `json:"password"  validate:"required,min=8,max=72"`
}

// RegisterResponse is returned on a successful registration.
//
// NOTE: verification_token is included here to allow testing without a live
// email service. In production this field must be removed from the response
// and the token delivered only via email.
type RegisterResponse struct {
	ID                string `json:"id"`
	Email             string `json:"email"`
	Username          string `json:"username"`
	Message           string `json:"message"`
	VerificationToken string `json:"verification_token,omitempty"` // dev only — stripped by handler in production (H2)
}

// ---- password reset ---------------------------------------------------------

// ForgotPasswordRequest is the payload for POST /api/v1/auth/forgot-password.
//
// The endpoint always returns HTTP 200 with the same message body regardless
// of whether the email is registered. This prevents user-enumeration: an
// attacker cannot distinguish "email not found" from "email found, token
// created" by observing the response.
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ForgotPasswordResponse is returned by POST /api/v1/auth/forgot-password.
//
// In development, reset_token is populated so engineers can test the flow
// without an email transport. In production the handler strips it; the token
// is delivered only via email.
type ForgotPasswordResponse struct {
	Message    string `json:"message"`
	ResetToken string `json:"reset_token,omitempty"`
}

// ResetPasswordRequest is the payload for POST /api/v1/auth/reset-password.
//
// token is the raw (unhashed) reset token from the email link.
// password is the new plaintext password (min 8, max 72 bytes).
type ResetPasswordRequest struct {
	Token    string `json:"token"    validate:"required"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

// ---- me ---------------------------------------------------------------------

// MeResponse is the payload returned by GET /api/v1/auth/me (service layer).
// The HTTP handler augments this with role and organization_id from the token.
type MeResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	FullName string `json:"full_name"`
	Status   string `json:"status"`
}
