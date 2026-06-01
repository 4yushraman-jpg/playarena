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
				UserID:         claims.UserID,
				OrganizationID: claims.OrganizationID,
				Role:           claims.Role,
				Email:          claims.Email,
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
