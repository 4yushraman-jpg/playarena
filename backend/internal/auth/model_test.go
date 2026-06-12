package auth_test

import (
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/auth"
)

// TestIsPlatformUser verifies that platform privilege is determined ONLY by an
// explicit platform scope (GP-1). Player and onboarding tokens also carry an
// empty organization ID but must never be treated as platform users.
func TestIsPlatformUser(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		user auth.AuthUser
		want bool
	}{
		{
			name: "platform scope",
			user: auth.AuthUser{Scope: auth.ScopePlatform, Role: "platform_admin"},
			want: true,
		},
		{
			name: "player scope (empty org, must NOT be platform)",
			user: auth.AuthUser{Scope: auth.ScopePlayer, PlayerProfileID: "p1"},
			want: false,
		},
		{
			name: "onboarding scope",
			user: auth.AuthUser{Scope: auth.ScopeOnboarding, Role: auth.OnboardingRole},
			want: false,
		},
		{
			name: "organizer scope",
			user: auth.AuthUser{Scope: auth.ScopeOrganizer, OrganizationID: "8a8c84c8-0000-0000-0000-000000000001", Role: "org_owner"},
			want: false,
		},
		{
			name: "empty scope is never platform",
			user: auth.AuthUser{Role: "platform_admin"},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.user.IsPlatformUser(); got != tc.want {
				t.Errorf("IsPlatformUser() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDeriveScope covers explicit-scope wins and least-privilege legacy
// inference. The critical invariant: an unknown empty-org token is NEVER
// inferred as platform.
func TestDeriveScope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		claims auth.JWTClaims
		want   string
	}{
		{
			name:   "explicit scope wins (player)",
			claims: auth.JWTClaims{Scope: auth.ScopePlayer, OrganizationID: ""},
			want:   auth.ScopePlayer,
		},
		{
			name:   "explicit scope wins even over org id",
			claims: auth.JWTClaims{Scope: auth.ScopePlatform, OrganizationID: "org-uuid"},
			want:   auth.ScopePlatform,
		},
		{
			name:   "legacy organizer: org id set",
			claims: auth.JWTClaims{OrganizationID: "org-uuid", Role: "org_owner"},
			want:   auth.ScopeOrganizer,
		},
		{
			name:   "legacy onboarding role",
			claims: auth.JWTClaims{OrganizationID: "", Role: auth.OnboardingRole},
			want:   auth.ScopeOnboarding,
		},
		{
			name:   "legacy platform: recognized platform role slug",
			claims: auth.JWTClaims{OrganizationID: "", Role: "platform_admin"},
			want:   auth.ScopePlatform,
		},
		{
			name:   "unknown empty-org token falls back to onboarding (least privilege)",
			claims: auth.JWTClaims{OrganizationID: "", Role: "some_unknown_role"},
			want:   auth.ScopeOnboarding,
		},
		{
			name:   "empty-org empty-role falls back to onboarding (never platform)",
			claims: auth.JWTClaims{OrganizationID: "", Role: ""},
			want:   auth.ScopeOnboarding,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := auth.DeriveScope(&tc.claims); got != tc.want {
				t.Errorf("DeriveScope() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsPlatformRoleSlug(t *testing.T) {
	t.Parallel()
	if !auth.IsPlatformRoleSlug("platform_admin") {
		t.Error("platform_admin should be a platform role slug")
	}
	if auth.IsPlatformRoleSlug("org_owner") {
		t.Error("org_owner must not be a platform role slug")
	}
	if auth.IsPlatformRoleSlug(auth.OnboardingRole) {
		t.Error("onboarding must not be a platform role slug")
	}
}
