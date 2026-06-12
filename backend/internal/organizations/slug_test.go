package organizations

import "testing"

// TestGenerateSlugReservedWords ensures org names that slugify to a reserved
// top-level route segment (e.g. "me", "players") are prefixed so they cannot
// shadow the GP-1 non-org routes.
func TestGenerateSlugReservedWords(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"me is reserved", "Me", "org-me"},
		{"players is reserved", "Players", "org-players"},
		{"onboarding is reserved", "Onboarding", "org-onboarding"},
		{"normal name unaffected", "Mumbai Raiders", "mumbai-raiders"},
		{"player singular reserved", "player", "org-player"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := generateSlug(tc.in); got != tc.want {
				t.Errorf("generateSlug(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
