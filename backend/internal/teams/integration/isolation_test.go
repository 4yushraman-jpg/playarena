package teams_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestTeam_Create_WrongOrg_BOLA verifies an actor from Org A cannot create a
// team under Org B's URL.
func TestTeam_Create_WrongOrg_BOLA(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, teamsURL(orgB.orgSlug), map[string]any{
		"name": "BOLA Team",
	}, bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestTeam_AddMember_WrongOrg_BOLA verifies an actor from Org A cannot add a
// member to Org B's team.
func TestTeam_AddMember_WrongOrg_BOLA(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgBUID := mustUUID(t, orgB.orgID)
	teamB := fixtures.CreateTeam(ctx, t, ts.pool, orgBUID)
	playerB := fixtures.CreatePlayer(ctx, t, ts.pool, orgBUID)
	teamBID := pgutil.UUIDToString(teamB.ID)
	playerBID := pgutil.UUIDToString(playerB.ID)

	resp := ts.postWithHeaders(t, membersURL(orgB.orgSlug, teamBID), map[string]any{
		"player_id": playerBID,
	}, bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestTeam_Get_WrongOrg_Invisible verifies a team from Org A is not accessible
// via Org B's URL.
func TestTeam_Get_WrongOrg_Invisible(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgAUID := mustUUID(t, orgA.orgID)
	teamA := fixtures.CreateTeam(ctx, t, ts.pool, orgAUID)
	teamAID := pgutil.UUIDToString(teamA.ID)

	// Org B actor tries to access Org A's team via Org B's slug.
	resp := ts.get(t, teamURL(orgB.orgSlug, teamAID), bearerHeader(orgB.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

// TestTeam_List_OrgScoped verifies GET /teams for Org A does not return teams
// belonging to Org B.
func TestTeam_List_OrgScoped(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgBUID := mustUUID(t, orgB.orgID)
	teamB := fixtures.CreateTeam(ctx, t, ts.pool, orgBUID)
	teamBID := pgutil.UUIDToString(teamB.ID)

	resp := ts.get(t, teamsURL(orgA.orgSlug), bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list teamListResponse
	decodeBody(t, resp, &list)
	for _, team := range list.Teams {
		if team.ID == teamBID {
			t.Errorf("Org B team %q appeared in Org A's team list", teamBID)
		}
	}
}
