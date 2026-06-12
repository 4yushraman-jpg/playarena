package players_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestPlayer_Create_NoAuth verifies POST without a token returns 401.
func TestPlayer_Create_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.post(t, playersURL(actor.orgSlug), map[string]any{
		"display_name": "Unauthorized Player",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

// TestPlayer_Create_NoPermission verifies a viewer token on POST returns 403.
func TestPlayer_Create_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	viewer := setupUserAndOrg(t, ts, "viewer")
	_ = viewer

	// viewer from their own org — needs player.create which viewers don't have
	// Use the actor's org slug (the one viewer would be trying to access)
	resp := ts.postWithHeaders(t, playersURL(actor.orgSlug), map[string]any{
		"display_name": "Should Not Create",
	}, bearerHeader(viewer.token))
	defer resp.Body.Close()
	// viewer lacks player.create permission → 403
	assertStatus(t, resp, http.StatusForbidden)
}

// TestPlayer_Delete_NoPermission verifies a viewer token on DELETE returns 403.
func TestPlayer_Delete_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	player := fixtures.CreatePlayer(ctx, t, ts.pool, orgUID)
	playerIDStr := pgutil.UUIDToString(player.ID)

	viewer := setupUserAndOrg(t, ts, "viewer")

	resp := ts.delete(t, playerURL(actor.orgSlug, playerIDStr), bearerHeader(viewer.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestPlayer_List_OnboardingToken_Forbidden verifies that a zero-org user's
// onboarding token cannot read another organization's data. Onboarding tokens
// carry an empty org ID (the same shape as platform admin tokens) and must be
// rejected by RequireOrgScope before reaching the tenant-exempt service layer.
func TestPlayer_List_OnboardingToken_Forbidden(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	zeroOrgUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, ts.pool, zeroOrgUser.ID) })

	// Login without organization_id — zero-org users receive an onboarding token.
	token := loginAs(t, ts, zeroOrgUser.Email, fixtures.KnownPasswordRaw, "")

	resp := ts.get(t, playersURL(actor.orgSlug), bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestPlayer_Get_NoAuth verifies GET /{id} without a token returns 401.
func TestPlayer_Get_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	player := fixtures.CreatePlayer(ctx, t, ts.pool, orgUID)
	playerIDStr := pgutil.UUIDToString(player.ID)

	resp := ts.get(t, playerURL(actor.orgSlug, playerIDStr), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}
