package teams_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestTeam_Create_NoAuth verifies POST /teams without a token returns 401.
func TestTeam_Create_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.post(t, teamsURL(actor.orgSlug), map[string]any{
		"name": "Unauthorized Team",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

// TestTeam_Create_NoPermission verifies that a viewer role cannot create teams.
func TestTeam_Create_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "viewer")

	resp := ts.postWithHeaders(t, teamsURL(actor.orgSlug), map[string]any{
		"name": "Should Fail",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestTeam_Update_NoPermission verifies that a same-org viewer cannot update a team.
// The viewer is in the same org as the owner so 403 comes from permission denial,
// not from BOLA.
func TestTeam_Update_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	owner := setupUserAndOrg(t, ts, "org_owner")
	orgUID := mustUUID(t, owner.orgID)

	viewerUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	fixtures.AddUserToOrg(ctx, t, ts.pool, orgUID, viewerUser.ID, "viewer")
	viewerToken := loginAs(t, ts, viewerUser.Email, fixtures.KnownPasswordRaw, owner.orgID)

	team := fixtures.CreateTeam(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)

	resp := ts.patch(t, teamURL(owner.orgSlug, teamID), map[string]any{
		"name": "Hijacked",
	}, bearerHeader(viewerToken))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestTeam_Delete_NoPermission verifies that a same-org viewer cannot disband a team.
func TestTeam_Delete_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	owner := setupUserAndOrg(t, ts, "org_owner")
	orgUID := mustUUID(t, owner.orgID)

	viewerUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	fixtures.AddUserToOrg(ctx, t, ts.pool, orgUID, viewerUser.ID, "viewer")
	viewerToken := loginAs(t, ts, viewerUser.Email, fixtures.KnownPasswordRaw, owner.orgID)

	team := fixtures.CreateTeam(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)

	resp := ts.delete(t, teamURL(owner.orgSlug, teamID), bearerHeader(viewerToken))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestTeam_AddMember_NoPermission verifies that a same-org viewer cannot add members.
func TestTeam_AddMember_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	owner := setupUserAndOrg(t, ts, "org_owner")
	orgUID := mustUUID(t, owner.orgID)

	viewerUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	fixtures.AddUserToOrg(ctx, t, ts.pool, orgUID, viewerUser.ID, "viewer")
	viewerToken := loginAs(t, ts, viewerUser.Email, fixtures.KnownPasswordRaw, owner.orgID)

	team := fixtures.CreateTeam(ctx, t, ts.pool, orgUID)
	player := fixtures.CreatePlayer(ctx, t, ts.pool, orgUID)
	teamID := pgutil.UUIDToString(team.ID)
	playerID := pgutil.UUIDToString(player.ID)

	resp := ts.postWithHeaders(t, membersURL(owner.orgSlug, teamID), map[string]any{
		"player_id": playerID,
	}, bearerHeader(viewerToken))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestTeam_List_NoAuth verifies GET /teams without a token returns 401.
func TestTeam_List_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, teamsURL(actor.orgSlug), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}
