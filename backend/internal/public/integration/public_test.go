package public_integration_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── visibility gate (404 matrix) ────────────────────────────────────────────────

func TestPublic_Unlisted_Accessible(t *testing.T) {
	ts := buildTestServer(t, testPool)
	s := seedOngoingTournament(t, ts.pool)
	setVisibility(t, ts.pool, s.tourn.ID, "unlisted")

	resp := ts.get(t, publicURL(s.org.Slug, s.tourn.Slug))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var out map[string]any
	decode(t, resp, &out)
	if out["name"] != s.tourn.Name {
		t.Errorf("name = %v, want %q", out["name"], s.tourn.Name)
	}
	if org, ok := out["organization"].(map[string]any); !ok || org["slug"] != s.org.Slug {
		t.Errorf("organization branding missing/incorrect: %v", out["organization"])
	}
}

func TestPublic_Public_Accessible(t *testing.T) {
	ts := buildTestServer(t, testPool)
	s := seedOngoingTournament(t, ts.pool)
	setVisibility(t, ts.pool, s.tourn.ID, "public")
	resp := ts.get(t, publicURL(s.org.Slug, s.tourn.Slug))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
}

func TestPublic_Private_Returns404(t *testing.T) {
	ts := buildTestServer(t, testPool)
	s := seedOngoingTournament(t, ts.pool)
	// Default visibility is 'private'.
	resp := ts.get(t, publicURL(s.org.Slug, s.tourn.Slug))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

func TestPublic_Draft_Returns404(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()
	org := fixtures.CreateOrg(ctx, t, ts.pool)
	tourn := fixtures.CreateTournament(ctx, t, ts.pool, org.ID, "draft")
	// Even explicitly public, a draft is never exposed.
	setVisibility(t, ts.pool, tourn.ID, "public")
	resp := ts.get(t, publicURL(org.Slug, tourn.Slug))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

func TestPublic_InactiveOrg_Returns404(t *testing.T) {
	ts := buildTestServer(t, testPool)
	s := seedOngoingTournament(t, ts.pool)
	setVisibility(t, ts.pool, s.tourn.ID, "public")
	setOrgStatus(t, ts.pool, s.org.ID, "suspended")
	resp := ts.get(t, publicURL(s.org.Slug, s.tourn.Slug))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

func TestPublic_Unknown_Returns404(t *testing.T) {
	ts := buildTestServer(t, testPool)
	resp := ts.get(t, publicURL("no-such-org", "no-such-tournament"))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

// ── privacy / whitelist ─────────────────────────────────────────────────────────

func TestPublic_Overview_NoPrivateFields(t *testing.T) {
	ts := buildTestServer(t, testPool)
	s := seedOngoingTournament(t, ts.pool)
	setVisibility(t, ts.pool, s.tourn.ID, "public")
	resp := ts.get(t, publicURL(s.org.Slug, s.tourn.Slug))
	body := rawBody(t, resp)

	// Note: registration_opens_at/closes_at (the registration WINDOW) is a
	// whitelisted public field; registration RECORDS/registrants are not.
	for _, forbidden := range []string{
		"created_by", "updated_by", "settings", "generation", "created_at",
		"email", "phone", "registered_by", "organization_id",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("public overview leaked forbidden field %q: %s", forbidden, body)
		}
	}
}

func TestPublic_Matches_NamesResolved_NoPrivateFields(t *testing.T) {
	ts := buildTestServer(t, testPool)
	s := seedOngoingTournament(t, ts.pool)
	setVisibility(t, ts.pool, s.tourn.ID, "public")
	// A completed match: home beats away.
	ctx := context.Background()
	fixtures.CreateCompletedMatch(ctx, t, ts.pool, s.org.ID, s.tourn.ID, s.homeTeam.ID, s.awayTeam.ID, 30, 20, s.homeTeam.ID)

	resp := ts.get(t, publicURL(s.org.Slug, s.tourn.Slug)+"/matches")
	body := rawBody(t, resp)

	if !strings.Contains(body, s.homeTeam.Name) || !strings.Contains(body, s.awayTeam.Name) {
		t.Errorf("participant names not resolved in public matches: %s", body)
	}
	for _, forbidden := range []string{"notes", "metadata", "recorded_by", "created_by"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("public matches leaked forbidden field %q", forbidden)
		}
	}
}

func TestPublic_Standings_Computed(t *testing.T) {
	ts := buildTestServer(t, testPool)
	s := seedOngoingTournament(t, ts.pool)
	setVisibility(t, ts.pool, s.tourn.ID, "public")
	ctx := context.Background()
	fixtures.CreateCompletedMatch(ctx, t, ts.pool, s.org.ID, s.tourn.ID, s.homeTeam.ID, s.awayTeam.ID, 30, 20, s.homeTeam.ID)

	resp := ts.get(t, publicURL(s.org.Slug, s.tourn.Slug)+"/standings")
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var out struct {
		Standings []struct {
			ParticipantName string `json:"participant_name"`
			Points          int    `json:"points"`
			Wins            int    `json:"wins"`
		} `json:"standings"`
	}
	decode(t, resp, &out)
	if len(out.Standings) != 2 {
		t.Fatalf("standings rows = %d, want 2", len(out.Standings))
	}
	top := out.Standings[0]
	if top.ParticipantName != s.homeTeam.Name || top.Wins != 1 || top.Points != 3 {
		t.Errorf("top standing = %+v, want %s 1W/3pts", top, s.homeTeam.Name)
	}
}

func TestPublic_Standings_Private_404(t *testing.T) {
	ts := buildTestServer(t, testPool)
	s := seedOngoingTournament(t, ts.pool) // private by default
	resp := ts.get(t, publicURL(s.org.Slug, s.tourn.Slug)+"/standings")
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

// ── cross-org isolation ─────────────────────────────────────────────────────────

func TestPublic_CrossOrg_Isolation(t *testing.T) {
	ts := buildTestServer(t, testPool)
	a := seedOngoingTournament(t, ts.pool)
	setVisibility(t, ts.pool, a.tourn.ID, "public")
	b := seedOngoingTournament(t, ts.pool)

	// Org B's slug + Org A's tournament slug must not resolve A's tournament.
	resp := ts.get(t, publicURL(b.org.Slug, a.tourn.Slug))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}
