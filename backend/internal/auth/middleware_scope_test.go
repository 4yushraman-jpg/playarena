package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

const testSecret = "test-secret-key-at-least-32-bytes-long!!"

func testCfg() *config.Config { return &config.Config{JWTSecret: testSecret} }

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

// token mints a signed access token for the given scope via the production path.
// An empty scope produces a legacy-shaped token (no scope claim) for
// compatibility testing.
func token(t *testing.T, orgID, role, scope, profileID string) string {
	t.Helper()
	tok, err := auth.GenerateAccessToken("11111111-1111-1111-1111-111111111111", orgID, role, "u@example.com", scope, profileID, testSecret)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	return tok
}

func runChain(t *testing.T, guard func(http.Handler) http.Handler, bearer string) int {
	t.Helper()
	h := auth.RequireAuth(testCfg())(guard(okHandler()))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// TestRequireOrgScope asserts only organizer and platform scopes pass; player
// and onboarding are rejected (tenant-isolation boundary).
func TestRequireOrgScope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		org, role, scp string
		want           int
	}{
		{"organizer passes", "22222222-2222-2222-2222-222222222222", "org_owner", auth.ScopeOrganizer, http.StatusOK},
		{"platform passes", "", "platform_admin", auth.ScopePlatform, http.StatusOK},
		{"player rejected", "", "", auth.ScopePlayer, http.StatusForbidden},
		{"onboarding rejected", "", auth.OnboardingRole, auth.ScopeOnboarding, http.StatusForbidden},
		{"legacy platform_admin (no scope) passes", "", "platform_admin", "", http.StatusOK},
		{"legacy onboarding (no scope) rejected", "", auth.OnboardingRole, "", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runChain(t, auth.RequireOrgScope(), token(t, tc.org, tc.role, tc.scp, ""))
			if got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestRequirePlayerScope asserts only player scope passes.
func TestRequirePlayerScope(t *testing.T) {
	t.Parallel()
	if got := runChain(t, auth.RequirePlayerScope(), token(t, "", "", auth.ScopePlayer, "p1")); got != http.StatusOK {
		t.Errorf("player scope: status = %d, want 200", got)
	}
	if got := runChain(t, auth.RequirePlayerScope(), token(t, "33333333-3333-3333-3333-333333333333", "org_owner", auth.ScopeOrganizer, "")); got != http.StatusForbidden {
		t.Errorf("organizer scope on player route: status = %d, want 403", got)
	}
	if got := runChain(t, auth.RequirePlayerScope(), token(t, "", auth.OnboardingRole, auth.ScopeOnboarding, "")); got != http.StatusForbidden {
		t.Errorf("onboarding scope on player route: status = %d, want 403", got)
	}
}

// TestRequireScope covers the generic guard.
func TestRequireScope(t *testing.T) {
	t.Parallel()
	guard := auth.RequireScope(auth.ScopePlatform, auth.ScopeOrganizer)
	if got := runChain(t, guard, token(t, "", "platform_admin", auth.ScopePlatform, "")); got != http.StatusOK {
		t.Errorf("platform allowed: got %d", got)
	}
	if got := runChain(t, guard, token(t, "", "", auth.ScopePlayer, "p1")); got != http.StatusForbidden {
		t.Errorf("player not in allow-set: got %d, want 403", got)
	}
}

// TestRequireAuthUnauthenticated ensures the chain 401s without a token.
func TestRequireAuthUnauthenticated(t *testing.T) {
	t.Parallel()
	h := auth.RequireAuth(testCfg())(auth.RequireOrgScope()(okHandler()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
