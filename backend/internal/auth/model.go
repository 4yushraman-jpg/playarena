package auth

import (
	"github.com/golang-jwt/jwt/v5"
)

const OnboardingRole = "onboarding"

// Persona scopes (GP-1). The active scope is carried explicitly in the JWT and
// resolved (for legacy tokens) by DeriveScope. It is the single authority for
// platform-admin determination — never infer privilege from an empty org ID.
const (
	ScopePlayer     = "player"
	ScopeOrganizer  = "organizer"
	ScopeOnboarding = "onboarding"
	ScopePlatform   = "platform"
)

// platformRoleSlugs is the closed set of role slugs that denote a platform-level
// principal. Used only for legacy (no-scope) token compatibility in DeriveScope.
// Newly issued platform tokens carry scope=platform explicitly.
var platformRoleSlugs = map[string]struct{}{
	"platform_admin": {},
}

// IsPlatformRoleSlug reports whether the given role slug is a platform role.
func IsPlatformRoleSlug(slug string) bool {
	_, ok := platformRoleSlugs[slug]
	return ok
}

// AuthUser is the decoded principal extracted from a validated access token.
// It is embedded into request context by the auth middleware.
type AuthUser struct {
	UserID          string `json:"user_id"`
	OrganizationID  string `json:"organization_id"` // empty for platform/player/onboarding tokens
	Role            string `json:"role"`
	Email           string `json:"email"`
	Scope           string `json:"scope"`             // player | organizer | onboarding | platform
	PlayerProfileID string `json:"player_profile_id"` // set when Scope == player
}

// IsPlatformUser reports whether this principal holds a platform-level token.
// GP-1: platform privilege is determined ONLY by an explicit platform scope.
// Player and onboarding tokens also carry an empty organization ID but must
// never receive platform privileges.
func (u *AuthUser) IsPlatformUser() bool {
	return u.Scope == ScopePlatform
}

// JWTClaims are the custom claims embedded in every access token.
// RegisteredClaims carries iss, sub, exp, iat, nbf.
type JWTClaims struct {
	UserID          string `json:"user_id"`
	OrganizationID  string `json:"organization_id"`
	Role            string `json:"role"`
	Email           string `json:"email"`
	Scope           string `json:"scope,omitempty"`             // GP-1
	PlayerProfileID string `json:"player_profile_id,omitempty"` // GP-1
	jwt.RegisteredClaims
}

// DeriveScope resolves the scope of any token, including legacy (pre-GP-1)
// tokens that carry no scope claim. Least-privilege on ambiguity:
//   - an explicit scope claim always wins;
//   - a non-empty organization_id means organizer;
//   - the onboarding role means onboarding;
//   - a recognized platform role slug means platform;
//   - anything else (e.g. an unknown empty-org token) falls back to onboarding,
//     NEVER platform — closing the "empty org ⇒ platform admin" footgun.
//
// Legacy tokens never carried scope=player (player tokens did not exist before
// GP-1), so this inference can never fabricate a player principal.
func DeriveScope(c *JWTClaims) string {
	if c.Scope != "" {
		return c.Scope
	}
	if c.OrganizationID != "" {
		return ScopeOrganizer
	}
	if c.Role == OnboardingRole {
		return ScopeOnboarding
	}
	if IsPlatformRoleSlug(c.Role) {
		return ScopePlatform
	}
	return ScopeOnboarding
}

// OrgSummary is the minimal organization representation returned to callers
// that need to make an organization selection (e.g., multi-org login).
type OrgSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}
