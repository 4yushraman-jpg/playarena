package tournament_registrations_integration_test

import (
	"context"
	"net/http"
	"testing"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestRegistration_Rule_TournamentNotRegistrationOpen verifies registering for a
// draft tournament returns 409/400 (registration closed error).
func TestRegistration_Rule_TournamentNotRegistrationOpen(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	tournament := fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusDraft)
	team, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamID := pgutil.UUIDToString(team.ID)

	resp := ts.postWithHeaders(t, registrationsURL(actor.orgSlug, tournamentID),
		map[string]any{"team_id": teamID},
		bearerHeader(actor.token))
	defer resp.Body.Close()
	// Should fail: tournament not in registration_open status.
	if resp.StatusCode != http.StatusConflict && resp.StatusCode != http.StatusBadRequest &&
		resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 4xx, got %d", resp.StatusCode)
	}
}

// TestRegistration_Rule_DuplicateRegistration verifies the same team cannot
// register twice for the same tournament.
func TestRegistration_Rule_DuplicateRegistration(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	team, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamID := pgutil.UUIDToString(team.ID)

	// First registration succeeds.
	r1 := ts.postWithHeaders(t, registrationsURL(actor.orgSlug, tournamentID),
		map[string]any{"team_id": teamID},
		bearerHeader(actor.token))
	r1.Body.Close()
	assertStatus(t, r1, http.StatusCreated)

	// Second registration must fail.
	r2 := ts.postWithHeaders(t, registrationsURL(actor.orgSlug, tournamentID),
		map[string]any{"team_id": teamID},
		bearerHeader(actor.token))
	defer r2.Body.Close()
	assertStatus(t, r2, http.StatusConflict)
}

// TestRegistration_Rule_TeamNoMembers verifies registering an empty team returns
// an error.
func TestRegistration_Rule_TeamNoMembers(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	team := fixtures.CreateTeam(ctx, t, ts.pool, orgUID) // No members!
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamID := pgutil.UUIDToString(team.ID)

	resp := ts.postWithHeaders(t, registrationsURL(actor.orgSlug, tournamentID),
		map[string]any{"team_id": teamID},
		bearerHeader(actor.token))
	defer resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for empty team, got %d", resp.StatusCode)
	}
}

// TestRegistration_Rule_DisbandedTeam verifies registering a disbanded team returns
// an error.
func TestRegistration_Rule_DisbandedTeam(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	disbandedTeam := fixtures.CreateDisbandedTeam(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamID := pgutil.UUIDToString(disbandedTeam.ID)

	resp := ts.postWithHeaders(t, registrationsURL(actor.orgSlug, tournamentID),
		map[string]any{"team_id": teamID},
		bearerHeader(actor.token))
	defer resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for disbanded team, got %d", resp.StatusCode)
	}
}

// TestRegistration_Rule_CrossOrgTeam verifies a team from a different org
// cannot register.
func TestRegistration_Rule_CrossOrgTeam(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgAUID := mustUUID(t, orgA.orgID)
	orgBUID := mustUUID(t, orgB.orgID)
	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgAUID)
	teamB, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgBUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamBID := pgutil.UUIDToString(teamB.ID)

	resp := ts.postWithHeaders(t, registrationsURL(orgA.orgSlug, tournamentID),
		map[string]any{"team_id": teamBID},
		bearerHeader(orgA.token))
	defer resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for cross-org team, got %d", resp.StatusCode)
	}
}

// TestRegistration_Rule_NoAuth verifies POST /registrations without a token returns 401.
func TestRegistration_Rule_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)

	resp := ts.post(t, registrationsURL(actor.orgSlug, tournamentID), map[string]any{
		"team_id": "00000000-0000-0000-0000-000000000001",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

// TestRegistration_Rule_CapacityEnforced verifies that registering more teams
// than max_participants returns 409 (tournament full).
func TestRegistration_Rule_CapacityEnforced(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	tournament := fixtures.CreateRegistrationOpenTournamentWithCapacity(ctx, t, ts.pool, orgUID, 2)
	tournamentID := pgutil.UUIDToString(tournament.ID)

	team1, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	team2, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	team3, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)

	// First two registrations must succeed.
	for _, teamID := range []string{pgutil.UUIDToString(team1.ID), pgutil.UUIDToString(team2.ID)} {
		resp := ts.postWithHeaders(t, registrationsURL(actor.orgSlug, tournamentID),
			map[string]any{"team_id": teamID},
			bearerHeader(actor.token))
		resp.Body.Close()
		assertStatus(t, resp, http.StatusCreated)
	}

	// Third registration must fail: tournament is full.
	resp := ts.postWithHeaders(t, registrationsURL(actor.orgSlug, tournamentID),
		map[string]any{"team_id": pgutil.UUIDToString(team3.ID)},
		bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusConflict)
}

// TestRegistration_Rule_InvalidStatusTransition verifies that an invalid status
// transition (e.g. rejected → approved) returns 400.
func TestRegistration_Rule_InvalidStatusTransition(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	team, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamID := pgutil.UUIDToString(team.ID)

	// Register.
	createResp := ts.postWithHeaders(t, registrationsURL(actor.orgSlug, tournamentID),
		map[string]any{"team_id": teamID},
		bearerHeader(actor.token))
	defer createResp.Body.Close()
	assertStatus(t, createResp, http.StatusCreated)
	var reg registrationResponse
	decodeBody(t, createResp, &reg)

	// Reject it.
	rejectResp := ts.patch(t, registrationURL(actor.orgSlug, tournamentID, reg.ID),
		map[string]any{"status": "rejected"},
		bearerHeader(actor.token))
	rejectResp.Body.Close()
	assertStatus(t, rejectResp, http.StatusOK)

	// Attempt to approve a rejected registration — must fail.
	resp := ts.patch(t, registrationURL(actor.orgSlug, tournamentID, reg.ID),
		map[string]any{"status": "approved"},
		bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnprocessableEntity)
}
