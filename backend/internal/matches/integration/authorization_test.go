package matches_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestMatch_Create_NoAuth verifies POST /matches without a token returns 401.
func TestMatch_Create_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.post(t, matchesURL(actor.orgSlug), map[string]any{
		"tournament_id": "00000000-0000-0000-0000-000000000001",
		"scheduled_at":  scheduledAt(),
	})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

// TestMatch_Create_NoPermission verifies a viewer cannot create matches.
func TestMatch_Create_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "viewer")

	resp := ts.postWithHeaders(t, matchesURL(actor.orgSlug), map[string]any{
		"tournament_id": "00000000-0000-0000-0000-000000000001",
		"scheduled_at":  scheduledAt(),
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestMatch_Update_NoPermission verifies a viewer cannot update matches.
func TestMatch_Update_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)

	ownerCtx := setupUserAndOrg(t, ts, "org_owner")
	viewerCtx := setupUserAndOrg(t, ts, "viewer")

	ctx := context.Background()
	orgUID := mustUUID(t, ownerCtx.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.patch(t, matchURL(ownerCtx.orgSlug, matchID), map[string]any{
		"status": "live",
	}, bearerHeader(viewerCtx.token))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

// TestMatch_Delete_NoPermission verifies a viewer cannot delete matches.
func TestMatch_Delete_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)

	ownerCtx := setupUserAndOrg(t, ts, "org_owner")
	viewerCtx := setupUserAndOrg(t, ts, "viewer")

	ctx := context.Background()
	orgUID := mustUUID(t, ownerCtx.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.delete(t, matchURL(ownerCtx.orgSlug, matchID), bearerHeader(viewerCtx.token))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

// TestMatch_List_NoAuth verifies GET /matches without a token returns 401.
func TestMatch_List_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.get(t, matchesURL(actor.orgSlug), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}
