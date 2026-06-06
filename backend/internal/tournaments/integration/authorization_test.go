package tournaments_integration_test

import (
	"context"
	"net/http"
	"testing"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestTournament_Create_NoAuth verifies POST /tournaments without a token returns 401.
func TestTournament_Create_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.post(t, tournamentsURL(actor.orgSlug), createTournamentPayload("No Auth"))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

// TestTournament_Create_NoPermission verifies a viewer cannot create tournaments.
func TestTournament_Create_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "viewer")

	resp := ts.postWithHeaders(t, tournamentsURL(actor.orgSlug),
		createTournamentPayload("Viewer Cup"),
		bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestTournament_Update_NoPermission verifies a viewer cannot update tournaments.
func TestTournament_Update_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)

	ownerCtx := setupUserAndOrg(t, ts, "org_owner")
	viewerCtx := setupUserAndOrg(t, ts, "viewer")

	ctx := context.Background()
	orgUID := mustUUID(t, ownerCtx.orgID)
	trmt := fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusDraft)
	trmtID := pgutil.UUIDToString(trmt.ID)

	resp := ts.patch(t, tournamentURL(ownerCtx.orgSlug, trmtID), map[string]any{
		"description": "Hijacked",
	}, bearerHeader(viewerCtx.token))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

// TestTournament_Delete_NoPermission verifies a viewer cannot delete tournaments.
func TestTournament_Delete_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)

	ownerCtx := setupUserAndOrg(t, ts, "org_owner")
	viewerCtx := setupUserAndOrg(t, ts, "viewer")

	ctx := context.Background()
	orgUID := mustUUID(t, ownerCtx.orgID)
	trmt := fixtures.CreateTournament(ctx, t, ts.pool, orgUID, db.TournamentStatusDraft)
	trmtID := pgutil.UUIDToString(trmt.ID)

	resp := ts.delete(t, tournamentURL(ownerCtx.orgSlug, trmtID), bearerHeader(viewerCtx.token))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

// TestTournament_List_NoAuth verifies GET /tournaments without a token returns 401.
func TestTournament_List_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, tournamentsURL(actor.orgSlug), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}
