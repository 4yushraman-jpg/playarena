package tournaments_integration_test

import (
	"context"
	"net/http"
	"testing"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestTournament_Create_WrongOrg_BOLA verifies an actor from Org A cannot create
// a tournament under Org B's URL.
func TestTournament_Create_WrongOrg_BOLA(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, tournamentsURL(orgB.orgSlug),
		createTournamentPayload("BOLA Cup"),
		bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestTournament_Update_WrongOrg_BOLA verifies Org A actor cannot update Org B's tournament.
func TestTournament_Update_WrongOrg_BOLA(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgBUID := mustUUID(t, orgB.orgID)
	trmtB := fixtures.CreateTournament(ctx, t, ts.pool, orgBUID, db.TournamentStatusDraft)
	trmtBID := pgutil.UUIDToString(trmtB.ID)

	resp := ts.patch(t, tournamentURL(orgB.orgSlug, trmtBID), map[string]any{
		"description": "Hijacked by Org A",
	}, bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestTournament_Get_WrongOrg_Invisible verifies a tournament from Org A is not
// accessible via Org B's URL.
func TestTournament_Get_WrongOrg_Invisible(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgAUID := mustUUID(t, orgA.orgID)
	trmtA := fixtures.CreateTournament(ctx, t, ts.pool, orgAUID, db.TournamentStatusDraft)
	trmtAID := pgutil.UUIDToString(trmtA.ID)

	// Org B actor accesses Org A's tournament via Org B's slug.
	resp := ts.get(t, tournamentURL(orgB.orgSlug, trmtAID), bearerHeader(orgB.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusNotFound)
}

// TestTournament_List_OrgScoped verifies GET /tournaments for Org A does not
// return tournaments belonging to Org B.
func TestTournament_List_OrgScoped(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgBUID := mustUUID(t, orgB.orgID)
	trmtB := fixtures.CreateTournament(ctx, t, ts.pool, orgBUID, db.TournamentStatusDraft)
	trmtBID := pgutil.UUIDToString(trmtB.ID)

	resp := ts.get(t, tournamentsURL(orgA.orgSlug), bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list tournamentListResponse
	decodeBody(t, resp, &list)
	for _, trmt := range list.Tournaments {
		if trmt.ID == trmtBID {
			t.Errorf("Org B tournament %q appeared in Org A's tournament list", trmtBID)
		}
	}
}
