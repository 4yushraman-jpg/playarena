package players_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestPlayer_Create_WrongOrg_BOLA verifies that an actor from Org A cannot
// create a player under Org B's URL.
func TestPlayer_Create_WrongOrg_BOLA(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, playersURL(orgB.orgSlug), map[string]any{
		"display_name": "BOLA Player",
	}, bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestPlayer_Get_WrongOrg_Invisible verifies a player from Org A is not
// accessible via Org B's URL.
func TestPlayer_Get_WrongOrg_Invisible(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgAUID := mustUUID(t, orgA.orgID)
	player := fixtures.CreatePlayer(ctx, t, ts.pool, orgAUID)
	playerIDStr := pgutil.UUIDToString(player.ID)

	// Org B actor tries to access Org A's player.
	resp := ts.get(t, playerURL(orgB.orgSlug, playerIDStr), bearerHeader(orgB.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

// TestPlayer_List_OrgScoped verifies GET /players for Org A does not return
// players belonging to Org B.
func TestPlayer_List_OrgScoped(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgBUID := mustUUID(t, orgB.orgID)
	playerB := fixtures.CreatePlayer(ctx, t, ts.pool, orgBUID)
	playerBIDStr := pgutil.UUIDToString(playerB.ID)

	// List players in Org A — Org B's player must not appear.
	resp := ts.get(t, playersURL(orgA.orgSlug), bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list playerListResponse
	decodeBody(t, resp, &list)
	for _, p := range list.Players {
		if p.ID == playerBIDStr {
			t.Errorf("Org B player %q appeared in Org A's player list", playerBIDStr)
		}
	}
}
