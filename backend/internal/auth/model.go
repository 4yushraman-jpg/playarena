package auth

import (
	"github.com/golang-jwt/jwt/v5"
)

// AuthUser is the decoded principal extracted from a validated access token.
// It is embedded into request context by the auth middleware.
type AuthUser struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"` // empty for platform-level tokens
	Role           string `json:"role"`
	Email          string `json:"email"`
}

// IsPlatformUser reports whether this principal has a platform-level token
// (i.e., no organization context).
func (u *AuthUser) IsPlatformUser() bool {
	return u.OrganizationID == ""
}

// JWTClaims are the custom claims embedded in every access token.
// RegisteredClaims carries iss, sub, exp, iat, nbf.
type JWTClaims struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	Role           string `json:"role"`
	Email          string `json:"email"`
	jwt.RegisteredClaims
}

// OrgSummary is the minimal organization representation returned to callers
// that need to make an organization selection (e.g., multi-org login).
type OrgSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}
