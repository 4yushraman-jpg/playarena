package teams_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestTeam_Create_EmptyName verifies name="" returns a 400 validation error.
func TestTeam_Create_EmptyName(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, teamsURL(actor.orgSlug), map[string]any{
		"name": "",
	}, bearerHeader(actor.token))
	assertValidationError(t, resp, "name")
}

// TestTeam_Create_MalformedJSON verifies syntactically invalid body returns 400.
func TestTeam_Create_MalformedJSON(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postRawWithHeaders(t, teamsURL(actor.orgSlug), `{"name":}`, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestTeam_Create_InvalidColor verifies an invalid hex color returns 400.
func TestTeam_Create_InvalidColor(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	color := "not-a-color"
	resp := ts.postWithHeaders(t, teamsURL(actor.orgSlug), map[string]any{
		"name":          "Color Team",
		"primary_color": color,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestTeam_AddMember_PlayerAlreadyAssigned verifies adding an already-assigned
// player returns 409.
func TestTeam_AddMember_PlayerAlreadyAssigned(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	team, player := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	team2 := fixtures.CreateTeam(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)
	team2ID := pgutil.UUIDToString(team2.ID)
	playerID := pgutil.UUIDToString(player.ID)

	// Player is already a member of team; try adding to team2.
	resp := ts.postWithHeaders(t, membersURL(actor.orgSlug, team2ID), map[string]any{
		"player_id": playerID,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusConflict)
	_ = teamID // suppress unused var
}

// TestTeam_AddMember_CrossOrgPlayer verifies adding a player from another org
// returns 422.
func TestTeam_AddMember_CrossOrgPlayer(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgAUID := mustUUID(t, orgA.orgID)
	orgBUID := mustUUID(t, orgB.orgID)
	teamA := fixtures.CreateTeam(ctx, t, ts.pool, orgAUID)
	playerB := fixtures.CreatePlayer(ctx, t, ts.pool, orgBUID)
	teamAID := pgutil.UUIDToString(teamA.ID)
	playerBID := pgutil.UUIDToString(playerB.ID)

	// Org A actor tries to add Org B's player to Org A's team.
	resp := ts.postWithHeaders(t, membersURL(orgA.orgSlug, teamAID), map[string]any{
		"player_id": playerBID,
	}, bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnprocessableEntity)
}

// TestTeam_AddMember_PlayerNotFound verifies adding a non-existent player
// returns 422 (cross-org check fires before 404).
func TestTeam_AddMember_PlayerNotFound(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	team := fixtures.CreateTeam(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)

	resp := ts.postWithHeaders(t, membersURL(actor.orgSlug, teamID), map[string]any{
		"player_id": "00000000-0000-0000-0000-000000000001",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	// Player not found in org → cross-org membership error (422) OR not found (404).
	if resp.StatusCode != http.StatusUnprocessableEntity && resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 422 or 404, got %d", resp.StatusCode)
	}
}

// TestTeam_RemoveMember_NonexistentMembership verifies removing a non-existent
// membership returns 404.
func TestTeam_RemoveMember_NonexistentMembership(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	team := fixtures.CreateTeam(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)

	resp := ts.delete(t, memberURL(actor.orgSlug, teamID, "00000000-0000-0000-0000-000000000001"),
		bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

// TestTeam_Get_NonexistentID verifies GET with a non-existent UUID returns 404.
func TestTeam_Get_NonexistentID(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, teamURL(actor.orgSlug, "00000000-0000-0000-0000-000000000001"),
		bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}
