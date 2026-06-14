package fixturegen_integration_test

import (
	"net/http"
	"testing"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

// TestGenerate_RoundRobin_DryRunThenConfirm proves dry-run previews without
// persisting, and confirm persists the full single round-robin.
func TestGenerate_RoundRobin_DryRunThenConfirm(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	tid, _ := seedClosedTournament(t, ts, mustUUID(t, actor.orgID), db.TournamentFormatRoundRobin, 6)

	// Dry run — preview only.
	resp := ts.postAuth(t, generateURL(actor.orgSlug, tid), map[string]any{
		"seed_strategy": "registration", "dry_run": true,
	}, actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var preview generateResponse
	decodeBody(t, resp, &preview)
	if !preview.DryRun || preview.Generated != 0 {
		t.Fatalf("dry run: DryRun=%v Generated=%d", preview.DryRun, preview.Generated)
	}
	if len(preview.Matches) != 15 { // C(6,2)
		t.Fatalf("preview matches = %d, want 15", len(preview.Matches))
	}
	if countMatches(t, ts, tid) != 0 {
		t.Fatal("dry run persisted matches (must not)")
	}

	// Confirm.
	resp2 := ts.postAuth(t, generateURL(actor.orgSlug, tid), map[string]any{
		"seed_strategy": "registration",
	}, actor.token)
	defer resp2.Body.Close()
	assertStatus(t, resp2, http.StatusCreated)
	var confirmed generateResponse
	decodeBody(t, resp2, &confirmed)
	if confirmed.Generated != 15 {
		t.Fatalf("generated = %d, want 15", confirmed.Generated)
	}
	if confirmed.GeneratedAt == nil {
		t.Fatal("expected generated_at on confirm")
	}
	if countMatches(t, ts, tid) != 15 {
		t.Fatalf("persisted matches = %d, want 15", countMatches(t, ts, tid))
	}
}

// TestGenerate_Knockout_Wiring proves a generated knockout is fully wired with
// next_match_id linkage (FE-8B), with exactly one unwired final.
func TestGenerate_Knockout_Wiring(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	tid, _ := seedClosedTournament(t, ts, mustUUID(t, actor.orgID), db.TournamentFormatKnockout, 8)

	resp := ts.postAuth(t, generateURL(actor.orgSlug, tid), map[string]any{"seed_strategy": "registration"}, actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	if got := countMatches(t, ts, tid); got != 7 {
		t.Fatalf("knockout matches = %d, want 7", got)
	}
	var linked, finals int
	rows, err := ts.pool.Query(t.Context(),
		"SELECT next_match_id IS NOT NULL FROM matches WHERE tournament_id = $1", mustUUID(t, tid))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var hasNext bool
		if err := rows.Scan(&hasNext); err != nil {
			t.Fatal(err)
		}
		if hasNext {
			linked++
		} else {
			finals++
		}
	}
	if linked != 6 || finals != 1 {
		t.Fatalf("wiring: linked=%d finals=%d, want 6/1", linked, finals)
	}
}

// TestGenerate_Safety_NotRegistrationClosed blocks generation outside reg_closed.
func TestGenerate_Safety_NotRegistrationClosed(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	tid, _ := seedClosedTournament(t, ts, mustUUID(t, actor.orgID), db.TournamentFormatKnockout, 8)
	setTournamentStatus(t, ts, tid, "ongoing")

	resp := ts.postAuth(t, generateURL(actor.orgSlug, tid), map[string]any{"seed_strategy": "registration"}, actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnprocessableEntity)
	if countMatches(t, ts, tid) != 0 {
		t.Fatal("blocked generation must not persist matches")
	}
}

// TestGenerate_Safety_FixturesExist blocks a second generation (no double-gen).
func TestGenerate_Safety_FixturesExist(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	tid, _ := seedClosedTournament(t, ts, mustUUID(t, actor.orgID), db.TournamentFormatKnockout, 8)

	first := ts.postAuth(t, generateURL(actor.orgSlug, tid), map[string]any{"seed_strategy": "registration"}, actor.token)
	first.Body.Close()
	assertStatus(t, first, http.StatusCreated)

	second := ts.postAuth(t, generateURL(actor.orgSlug, tid), map[string]any{"seed_strategy": "registration"}, actor.token)
	defer second.Body.Close()
	assertStatus(t, second, http.StatusUnprocessableEntity)
	if countMatches(t, ts, tid) != 7 {
		t.Fatalf("re-gen changed match count to %d, want 7", countMatches(t, ts, tid))
	}
}

// TestGenerate_Safety_NoPermission rejects a viewer.
func TestGenerate_Safety_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	owner := setupUserAndOrg(t, ts, "org_owner")
	viewer := setupUserAndOrg(t, ts, "viewer")
	tid, _ := seedClosedTournament(t, ts, mustUUID(t, owner.orgID), db.TournamentFormatKnockout, 8)

	resp := ts.postAuth(t, generateURL(owner.orgSlug, tid), map[string]any{"seed_strategy": "registration"}, viewer.token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

// TestGenerate_LateRegistration detects an approved-set change since preview.
func TestGenerate_LateRegistration(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	tid, _ := seedClosedTournament(t, ts, mustUUID(t, actor.orgID), db.TournamentFormatKnockout, 8)

	// Confirm but claim a different previewed count → ErrRegistrationsChanged.
	resp := ts.postAuth(t, generateURL(actor.orgSlug, tid), map[string]any{
		"seed_strategy": "registration", "expected_participants": 6,
	}, actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnprocessableEntity)
	if countMatches(t, ts, tid) != 0 {
		t.Fatal("mismatched-expectation generation must not persist")
	}
}

// TestGenerate_Explainability + reproducible random seed.
func TestGenerate_Explainability_Random(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	tid, _ := seedClosedTournament(t, ts, mustUUID(t, actor.orgID), db.TournamentFormatKnockout, 8)

	// Preview random → server returns the seed it used.
	prev := ts.postAuth(t, generateURL(actor.orgSlug, tid), map[string]any{
		"seed_strategy": "random", "dry_run": true,
	}, actor.token)
	defer prev.Body.Close()
	assertStatus(t, prev, http.StatusOK)
	var p1 generateResponse
	decodeBody(t, prev, &p1)
	if p1.RandomSeed == nil {
		t.Fatal("random preview did not return a seed")
	}

	// Confirm with that seed → bracket reproducible from it.
	conf := ts.postAuth(t, generateURL(actor.orgSlug, tid), map[string]any{
		"seed_strategy": "random", "random_seed": *p1.RandomSeed,
	}, actor.token)
	defer conf.Body.Close()
	assertStatus(t, conf, http.StatusCreated)
	var c1 generateResponse
	decodeBody(t, conf, &c1)
	if c1.RandomSeed == nil || *c1.RandomSeed != *p1.RandomSeed {
		t.Fatal("confirm did not echo the random seed")
	}
	// Same seed ⇒ same seed order (reproducible).
	if len(c1.SeedOrder) != len(p1.SeedOrder) {
		t.Fatal("seed order length mismatch")
	}
	for i := range c1.SeedOrder {
		if c1.SeedOrder[i] != p1.SeedOrder[i] {
			t.Fatalf("seed order differs at %d: %s vs %s", i, c1.SeedOrder[i], p1.SeedOrder[i])
		}
	}

	// Explainability record exposes the audit trail.
	info := ts.getAuth(t, generationURL(actor.orgSlug, tid), actor.token)
	defer info.Body.Close()
	assertStatus(t, info, http.StatusOK)
	var gi generationInfo
	decodeBody(t, info, &gi)
	if !gi.Generated || gi.Format != "knockout" || gi.SeedStrategy != "random" {
		t.Fatalf("generation info wrong: %+v", gi)
	}
	if gi.RandomSeed == nil || *gi.RandomSeed != *p1.RandomSeed {
		t.Fatal("generation info missing/incorrect random seed")
	}
	if gi.GeneratedBy == "" || gi.GeneratedAt == "" || gi.MatchCount != 7 {
		t.Fatalf("generation info incomplete: %+v", gi)
	}
}
