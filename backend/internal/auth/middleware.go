package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/response"
)

// ctxKey is a private context-key type that prevents key collisions with other
// packages that also store values in request contexts.
type ctxKey string

const ctxKeyAuthUser ctxKey = "auth_user"

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
