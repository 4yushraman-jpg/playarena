package tournament_registrations_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestRegistration_Register_Success verifies POST /registrations returns 201
// for a team registering in a registration_open tournament.
func TestRegistration_Register_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	team, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamID := pgutil.UUIDToString(team.ID)

	resp := ts.postWithHeaders(t, registrationsURL(actor.orgSlug, tournamentID),
		map[string]any{"team_id": teamID},
		bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	var reg registrationResponse
	decodeBody(t, resp, &reg)
	if reg.ID == "" {
		t.Error("expected registration ID in response")
	}
	if reg.Status != "pending" {
		t.Errorf("status = %q, want pending", reg.Status)
	}
	if reg.TournamentID != tournamentID {
		t.Errorf("tournament_id = %q, want %q", reg.TournamentID, tournamentID)
	}
	if reg.TeamID == nil || *reg.TeamID != teamID {
		t.Errorf("team_id = %v, want %q", reg.TeamID, teamID)
	}
}

// TestRegistration_List_Default verifies GET /registrations returns a paginated list.
func TestRegistration_List_Default(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)

	team1, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	team2, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)

	// Register both teams.
	for _, teamID := range []string{pgutil.UUIDToString(team1.ID), pgutil.UUIDToString(team2.ID)} {
		resp := ts.postWithHeaders(t, registrationsURL(actor.orgSlug, tournamentID),
			map[string]any{"team_id": teamID},
			bearerHeader(actor.token))
		resp.Body.Close()
	}

	listResp := ts.get(t, registrationsURL(actor.orgSlug, tournamentID), bearerHeader(actor.token))
	defer listResp.Body.Close()
	assertStatus(t, listResp, http.StatusOK)

	var list registrationListResponse
	decodeBody(t, listResp, &list)
	if list.Total < 2 {
		t.Errorf("total = %d, want >= 2", list.Total)
	}
}

// TestRegistration_GetByID_Success verifies GET /registrations/{id} returns the registration.
func TestRegistration_GetByID_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	team, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamID := pgutil.UUIDToString(team.ID)

	createResp := ts.postWithHeaders(t, registrationsURL(actor.orgSlug, tournamentID),
		map[string]any{"team_id": teamID},
		bearerHeader(actor.token))
	defer createResp.Body.Close()
	assertStatus(t, createResp, http.StatusCreated)
	var reg registrationResponse
	decodeBody(t, createResp, &reg)

	resp := ts.get(t, registrationURL(actor.orgSlug, tournamentID, reg.ID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got registrationResponse
	decodeBody(t, resp, &got)
	if got.ID != reg.ID {
		t.Errorf("id = %q, want %q", got.ID, reg.ID)
	}
}

// TestRegistration_Approve_Success verifies PATCH /registrations/{id} with
// status=approved transitions pending→approved.
func TestRegistration_Approve_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	team, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamID := pgutil.UUIDToString(team.ID)

	createResp := ts.postWithHeaders(t, registrationsURL(actor.orgSlug, tournamentID),
		map[string]any{"team_id": teamID},
		bearerHeader(actor.token))
	defer createResp.Body.Close()
	assertStatus(t, createResp, http.StatusCreated)
	var reg registrationResponse
	decodeBody(t, createResp, &reg)

	resp := ts.patch(t, registrationURL(actor.orgSlug, tournamentID, reg.ID), map[string]any{
		"status": "approved",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got registrationResponse
	decodeBody(t, resp, &got)
	if got.Status != "approved" {
		t.Errorf("status = %q, want approved", got.Status)
	}
}

// TestRegistration_Delete_Withdraw verifies DELETE /registrations/{id} sets
// status=withdrawn.
func TestRegistration_Delete_Withdraw(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	team, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamID := pgutil.UUIDToString(team.ID)

	createResp := ts.postWithHeaders(t, registrationsURL(actor.orgSlug, tournamentID),
		map[string]any{"team_id": teamID},
		bearerHeader(actor.token))
	defer createResp.Body.Close()
	assertStatus(t, createResp, http.StatusCreated)
	var reg registrationResponse
	decodeBody(t, createResp, &reg)

	resp := ts.delete(t, registrationURL(actor.orgSlug, tournamentID, reg.ID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNoContent)

	// Registration still retrievable.
	getResp := ts.get(t, registrationURL(actor.orgSlug, tournamentID, reg.ID), bearerHeader(actor.token))
	defer getResp.Body.Close()
	assertStatus(t, getResp, http.StatusOK)
	var got registrationResponse
	decodeBody(t, getResp, &got)
	if got.Status != "withdrawn" {
		t.Errorf("status = %q, want withdrawn", got.Status)
	}
}
