package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/response"
)

// ctxKey is a private context-key type that prevents key collisions with other
// packages that also store values in request contexts.
type ctxKey string

const ctxKeyAuthUser ctxKey = "auth_user"

// ---- JWT authentication middleware ------------------------------------------

// RequireAuth returns a chi-compatible middleware that:
//  1. Reads the Authorization header and expects "Bearer <token>"
//  2. Validates the JWT signature, algorithm, issuer, and expiry
//  3. Builds an *AuthUser from the validated claims
//  4. Stores the *AuthUser in the request context
//
// Any request that does not carry a valid access token receives a 401 response
// and the chain is halted. Error details are intentionally generic to avoid
// leaking information about why a token is invalid.
func RequireAuth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString, ok := bearerToken(r)
			if !ok {
				response.Error(w, http.StatusUnauthorized, "authorization required")
				return
			}

			claims, err := ValidateToken(tokenString, cfg.JWTSecret)
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "authorization required")
				return
			}

			principal := &AuthUser{
				UserID:          claims.UserID,
				OrganizationID:  claims.OrganizationID,
				Role:            claims.Role,
				Email:           claims.Email,
				Scope:           DeriveScope(claims),
				PlayerProfileID: claims.PlayerProfileID,
			}

			ctx := context.WithValue(r.Context(), ctxKeyAuthUser, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetAuthUser retrieves the *AuthUser stored by RequireAuth from ctx.
// Returns nil when called outside an authenticated request (e.g. tests that
// bypass the middleware). Callers should check for nil before use.
func GetAuthUser(ctx context.Context) *AuthUser {
	u, _ := ctx.Value(ctxKeyAuthUser).(*AuthUser)
	return u
}

// ---- Org-scope guard middleware -----------------------------------------------

// RequireOrgScope returns a middleware that admits only org-acting principals
// into org-scoped route trees.
//
// GP-1: only organizer and platform scopes may pass. Platform admins administer
// all orgs (org services exempt them from tenant ownership checks); organizers
// act within their own org. Player and onboarding tokens carry an empty
// OrganizationID — the same shape org services treat as a platform-admin
// exemption — so they MUST never reach those services. Mount this after
// RequireAuth on every route tree under /api/v1/organizations/{slug}/.
func RequireOrgScope() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := GetAuthUser(r.Context())
			if principal == nil {
				response.Error(w, http.StatusUnauthorized, "authorization required")
				return
			}
			if principal.Scope != ScopeOrganizer && principal.Scope != ScopePlatform {
				response.Error(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireScope returns a middleware that passes only when the authenticated
// principal's scope is one of the allowed scopes. Returns 401 when unauthenticated
// and 403 when the scope is not permitted.
func RequireScope(allowed ...string) func(http.Handler) http.Handler {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, s := range allowed {
		allowedSet[s] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := GetAuthUser(r.Context())
			if principal == nil {
				response.Error(w, http.StatusUnauthorized, "authorization required")
				return
			}
			if _, ok := allowedSet[principal.Scope]; !ok {
				response.Error(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePlayerScope admits only player-scope principals. Reserved for
// player-only feature routes. GP-1 self-profile routes use service-layer
// ownership checks (user_id == actor) rather than scope gating, so that an
// organizer/onboarding/platform user can still create and view their own
// profile.
func RequirePlayerScope() func(http.Handler) http.Handler {
	return RequireScope(ScopePlayer)
}

// ---- Role authorization middleware ------------------------------------------

// RequireRole returns a chi-compatible middleware that passes only when the
// authenticated user holds at least one of the given role slugs in their
// current org context.
//
// RequireAuth MUST be mounted before RequireRole in the middleware chain.
// If the request has no authenticated principal the middleware returns 401.
// If the user is authenticated but lacks the required role it returns 403.
//
// The org context is read from the AuthUser embedded in the JWT (which was
// resolved at login time). No additional DB round trip is needed for the
// token itself, but the DB IS queried to evaluate role grants.
func RequireRole(authz *AuthorizationService, roleSlugs ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := GetAuthUser(r.Context())
			if principal == nil {
				response.Error(w, http.StatusUnauthorized, "authorization required")
				return
			}

			ok, err := authz.HasRole(r.Context(), principal.UserID, principal.OrganizationID, roleSlugs...)
			if err != nil {
				slog.ErrorContext(r.Context(), "auth.require_role.error",
					slog.Any("error", err),
					slog.String("request_id", chimw.GetReqID(r.Context())),
				)
				response.Error(w, http.StatusInternalServerError, "internal server error")
				return
			}
			if !ok {
				response.Error(w, http.StatusForbidden, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ---- Permission authorization middleware ------------------------------------

// RequirePermission returns a chi-compatible middleware that passes only when
// the authenticated user holds the specified permission slug in their current
// org context.
//
// RequireAuth MUST be mounted before RequirePermission in the middleware chain.
// If the request has no authenticated principal the middleware returns 401.
// If the user lacks the permission it returns 403.
//
// Uses a single EXISTS query under the hood — no N+1.
func RequirePermission(authz *AuthorizationService, permSlug string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := GetAuthUser(r.Context())
			if principal == nil {
				response.Error(w, http.StatusUnauthorized, "authorization required")
				return
			}

			if permSlug == "organization.create" &&
				principal.OrganizationID == "" &&
				principal.Role == OnboardingRole {
				ok, err := authz.IsZeroOrgUser(r.Context(), principal.UserID)
				if err != nil {
					slog.ErrorContext(r.Context(), "auth.require_permission.onboarding_error",
						slog.Any("error", err),
						slog.String("request_id", chimw.GetReqID(r.Context())),
					)
					response.Error(w, http.StatusInternalServerError, "internal server error")
					return
				}
				if ok {
					next.ServeHTTP(w, r)
					return
				}
			}

			ok, err := authz.HasPermission(r.Context(), principal.UserID, principal.OrganizationID, permSlug)
			if err != nil {
				slog.ErrorContext(r.Context(), "auth.require_permission.error",
					slog.Any("error", err),
					slog.String("perm", permSlug),
					slog.String("request_id", chimw.GetReqID(r.Context())),
				)
				response.Error(w, http.StatusInternalServerError, "internal server error")
				return
			}
			if !ok {
				response.Error(w, http.StatusForbidden, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ---- helpers ----------------------------------------------------------------

// bearerToken extracts the raw JWT string from the Authorization header.
// Returns ("", false) when the header is absent or malformed.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	tok := strings.TrimPrefix(h, prefix)
	if tok == "" {
		return "", false
	}
	return tok, true
}
