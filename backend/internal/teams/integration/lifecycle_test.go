package teams_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestTeam_Create_Success verifies POST /teams returns 201 with team fields.
func TestTeam_Create_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, teamsURL(actor.orgSlug), map[string]any{
		"name": "Test Raiders",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	var team teamResponse
	decodeBody(t, resp, &team)
	if team.ID == "" {
		t.Error("expected team ID in response")
	}
	if team.Name != "Test Raiders" {
		t.Errorf("name = %q, want %q", team.Name, "Test Raiders")
	}
	if team.Slug == "" {
		t.Error("expected non-empty slug")
	}
	if team.Status != "active" {
		t.Errorf("status = %q, want active", team.Status)
	}
	if team.OrganizationID != actor.orgID {
		t.Errorf("organization_id = %q, want %q", team.OrganizationID, actor.orgID)
	}
}

// TestTeam_Create_SlugCollision verifies that creating two teams with the same
// name within one org generates distinct slugs (slug-2, slug-3, etc.).
func TestTeam_Create_SlugCollision(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	for i := 0; i < 3; i++ {
		resp := ts.postWithHeaders(t, teamsURL(actor.orgSlug), map[string]any{
			"name": "Collision Team",
		}, bearerHeader(actor.token))
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusCreated)
	}
}

// TestTeam_List_Default verifies GET /teams returns a paginated list.
func TestTeam_List_Default(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	fixtures.CreateTeam(ctx, t, ts.pool, orgUID)
	fixtures.CreateTeam(ctx, t, ts.pool, orgUID)

	resp := ts.get(t, teamsURL(actor.orgSlug), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list teamListResponse
	decodeBody(t, resp, &list)
	if list.Total < 2 {
		t.Errorf("total = %d, want >= 2", list.Total)
	}
	if len(list.Teams) < 2 {
		t.Errorf("len(teams) = %d, want >= 2", len(list.Teams))
	}
}

// TestTeam_Get_ActiveTeam verifies GET /teams/{id} returns an active team.
func TestTeam_Get_ActiveTeam(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	team := fixtures.CreateTeam(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)

	resp := ts.get(t, teamURL(actor.orgSlug, teamID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got teamResponse
	decodeBody(t, resp, &got)
	if got.ID != teamID {
		t.Errorf("id = %q, want %q", got.ID, teamID)
	}
	if got.Status != "active" {
		t.Errorf("status = %q, want active", got.Status)
	}
}

// TestTeam_Get_DisbandedTeam verifies GET /teams/{id} returns disbanded teams too
// (disbanded teams remain accessible for historical data).
func TestTeam_Get_DisbandedTeam(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	team := fixtures.CreateDisbandedTeam(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)

	resp := ts.get(t, teamURL(actor.orgSlug, teamID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got teamResponse
	decodeBody(t, resp, &got)
	if got.Status != "disbanded" {
		t.Errorf("status = %q, want disbanded", got.Status)
	}
}

// TestTeam_Update_Name verifies PATCH /teams/{id} updates the team name.
func TestTeam_Update_Name(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	team := fixtures.CreateTeam(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)

	resp := ts.patch(t, teamURL(actor.orgSlug, teamID), map[string]any{
		"name": "Updated Name",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got teamResponse
	decodeBody(t, resp, &got)
	if got.Name != "Updated Name" {
		t.Errorf("name = %q, want %q", got.Name, "Updated Name")
	}
}

// TestTeam_Delete_SoftDisband verifies DELETE /teams/{id} sets status=disbanded
// but the team record remains retrievable.
func TestTeam_Delete_SoftDisband(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	team := fixtures.CreateTeam(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)

	resp := ts.delete(t, teamURL(actor.orgSlug, teamID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNoContent)

	// Team still retrievable with disbanded status.
	getResp := ts.get(t, teamURL(actor.orgSlug, teamID), bearerHeader(actor.token))
	defer getResp.Body.Close()
	assertStatus(t, getResp, http.StatusOK)
	var got teamResponse
	decodeBody(t, getResp, &got)
	if got.Status != "disbanded" {
		t.Errorf("status = %q, want disbanded", got.Status)
	}
}

// TestTeam_AddMember_Success verifies POST /teams/{id}/members returns 201 with membership fields.
func TestTeam_AddMember_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	team := fixtures.CreateTeam(ctx, t, ts.pool, orgUID)
	player := fixtures.CreatePlayer(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)
	playerID := pgutil.UUIDToString(player.ID)

	resp := ts.postWithHeaders(t, membersURL(actor.orgSlug, teamID), map[string]any{
		"player_id": playerID,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	var m membershipResponse
	decodeBody(t, resp, &m)
	if m.ID == "" {
		t.Error("expected membership ID in response")
	}
	if m.PlayerID != playerID {
		t.Errorf("player_id = %q, want %q", m.PlayerID, playerID)
	}
	if m.Status != "active" {
		t.Errorf("status = %q, want active", m.Status)
	}
}

// TestTeam_ListMembers_ActiveOnly verifies GET /teams/{id}/members returns only
// active memberships.
func TestTeam_ListMembers_ActiveOnly(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	team, player := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)
	playerID := pgutil.UUIDToString(player.ID)

	resp := ts.get(t, membersURL(actor.orgSlug, teamID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list memberListResponse
	decodeBody(t, resp, &list)
	if len(list.Members) < 1 {
		t.Fatalf("expected at least 1 member, got %d", len(list.Members))
	}
	found := false
	for _, m := range list.Members {
		if m.PlayerID == playerID {
			found = true
			if m.Status != "active" {
				t.Errorf("member status = %q, want active", m.Status)
			}
		}
	}
	if !found {
		t.Errorf("player %q not found in member list", playerID)
	}
}

// TestTeam_RemoveMember_Success verifies DELETE /teams/{id}/members/{membershipId}
// returns 204.
func TestTeam_RemoveMember_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	team, player := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)

	// Get the membership ID.
	listResp := ts.get(t, membersURL(actor.orgSlug, teamID), bearerHeader(actor.token))
	defer listResp.Body.Close()
	assertStatus(t, listResp, http.StatusOK)
	var list memberListResponse
	decodeBody(t, listResp, &list)
	var membershipID string
	playerID := pgutil.UUIDToString(player.ID)
	for _, m := range list.Members {
		if m.PlayerID == playerID {
			membershipID = m.ID
			break
		}
	}
	if membershipID == "" {
		t.Fatal("membership not found in list")
	}

	resp := ts.delete(t, memberURL(actor.orgSlug, teamID, membershipID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNoContent)
}

// TestTeam_RemoveMember_StampedLeftAt verifies that after removal the membership
// is no longer in the active member list (left_at is set).
func TestTeam_RemoveMember_StampedLeftAt(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	team, player := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)
	playerID := pgutil.UUIDToString(player.ID)

	// Get the membership ID.
	listResp := ts.get(t, membersURL(actor.orgSlug, teamID), bearerHeader(actor.token))
	defer listResp.Body.Close()
	assertStatus(t, listResp, http.StatusOK)
	var list memberListResponse
	decodeBody(t, listResp, &list)
	var membershipID string
	for _, m := range list.Members {
		if m.PlayerID == playerID {
			membershipID = m.ID
			break
		}
	}
	if membershipID == "" {
		t.Fatal("membership not found in list")
	}

	// Remove the member.
	delResp := ts.delete(t, memberURL(actor.orgSlug, teamID, membershipID), bearerHeader(actor.token))
	defer delResp.Body.Close()
	assertStatus(t, delResp, http.StatusNoContent)

	// Member should no longer appear in the active member list.
	afterResp := ts.get(t, membersURL(actor.orgSlug, teamID), bearerHeader(actor.token))
	defer afterResp.Body.Close()
	assertStatus(t, afterResp, http.StatusOK)
	var after memberListResponse
	decodeBody(t, afterResp, &after)
	for _, m := range after.Members {
		if m.PlayerID == playerID {
			t.Errorf("removed player %q still appears in active member list", playerID)
		}
	}
}
