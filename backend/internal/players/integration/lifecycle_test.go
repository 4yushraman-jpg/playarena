package players_integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

func playersURL(orgSlug string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/players", orgSlug)
}

func playerURL(orgSlug, playerID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/players/%s", orgSlug, playerID)
}

// mustUUID parses a UUID string into pgtype.UUID; fails the test on error.
func mustUUID(t testing.TB, s string) pgtype.UUID {
	t.Helper()
	uid, err := pgutil.ParseUUID(s)
	if err != nil {
		t.Fatalf("mustUUID %q: %v", s, err)
	}
	return uid
}

// TestPlayer_Create_Success verifies POST returns 201 with required fields.
func TestPlayer_Create_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := ts.postWithHeaders(t, playersURL(actor.orgSlug), map[string]any{
		"display_name": "Arjun Singh",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	var body playerResponse
	decodeBody(t, resp, &body)
	if body.ID == "" {
		t.Error("expected non-empty id")
	}
	if body.Status != "active" {
		t.Errorf("status: got %q, want active", body.Status)
	}
	if body.DisplayName != "Arjun Singh" {
		t.Errorf("display_name: got %q", body.DisplayName)
	}
}

// TestPlayer_List_Default verifies GET returns players in the org.
func TestPlayer_List_Default(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	fixtures.CreatePlayer(ctx, t, ts.pool, orgUID)
	fixtures.CreatePlayer(ctx, t, ts.pool, orgUID)

	resp := ts.get(t, playersURL(actor.orgSlug), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list playerListResponse
	decodeBody(t, resp, &list)
	if list.Total < 2 {
		t.Errorf("total: want >= 2, got %d", list.Total)
	}
}

// TestPlayer_List_StatusFilter verifies ?status=inactive returns only inactive players.
func TestPlayer_List_StatusFilter(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	inactive := fixtures.CreateInactivePlayer(ctx, t, ts.pool, orgUID)
	fixtures.CreatePlayer(ctx, t, ts.pool, orgUID) // active — should not appear

	resp := ts.get(t, playersURL(actor.orgSlug)+"?status=inactive", bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list playerListResponse
	decodeBody(t, resp, &list)
	inactiveIDStr := pgutil.UUIDToString(inactive.ID)
	found := false
	for _, p := range list.Players {
		if p.ID == inactiveIDStr {
			found = true
		}
		if p.Status == "active" {
			t.Errorf("active player %q appeared in ?status=inactive list", p.ID)
		}
	}
	if !found {
		t.Errorf("inactive player %q not found in filtered list", inactiveIDStr)
	}
}

// TestPlayer_Get_ActivePlayer verifies GET /{id} returns 200 for an active player.
func TestPlayer_Get_ActivePlayer(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	player := fixtures.CreatePlayer(ctx, t, ts.pool, orgUID)
	playerIDStr := pgutil.UUIDToString(player.ID)

	resp := ts.get(t, playerURL(actor.orgSlug, playerIDStr), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var body playerResponse
	decodeBody(t, resp, &body)
	if body.ID != playerIDStr {
		t.Errorf("id: got %q, want %q", body.ID, playerIDStr)
	}
}

// TestPlayer_Get_InactivePlayer verifies GET /{id} returns 200 for a soft-deleted
// player — records must remain accessible for historical lookups.
func TestPlayer_Get_InactivePlayer(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	player := fixtures.CreateInactivePlayer(ctx, t, ts.pool, orgUID)
	playerIDStr := pgutil.UUIDToString(player.ID)

	resp := ts.get(t, playerURL(actor.orgSlug, playerIDStr), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var body playerResponse
	decodeBody(t, resp, &body)
	if body.Status != "inactive" {
		t.Errorf("status: got %q, want inactive", body.Status)
	}
}

// TestPlayer_Update_Success verifies PATCH updates the specified fields.
func TestPlayer_Update_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	player := fixtures.CreatePlayer(ctx, t, ts.pool, orgUID)
	playerIDStr := pgutil.UUIDToString(player.ID)

	resp := ts.patch(t, playerURL(actor.orgSlug, playerIDStr), map[string]any{
		"display_name": "Updated Name",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var body playerResponse
	decodeBody(t, resp, &body)
	if body.DisplayName != "Updated Name" {
		t.Errorf("display_name: got %q, want Updated Name", body.DisplayName)
	}
}

// TestPlayer_Delete_SoftDelete verifies DELETE sets status to inactive and the
// player remains accessible by ID.
func TestPlayer_Delete_SoftDelete(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	player := fixtures.CreatePlayer(ctx, t, ts.pool, orgUID)
	playerIDStr := pgutil.UUIDToString(player.ID)

	del := ts.delete(t, playerURL(actor.orgSlug, playerIDStr), bearerHeader(actor.token))
	defer del.Body.Close()
	assertStatus(t, del, http.StatusNoContent)

	get := ts.get(t, playerURL(actor.orgSlug, playerIDStr), bearerHeader(actor.token))
	defer get.Body.Close()
	assertStatus(t, get, http.StatusOK)
	var body playerResponse
	decodeBody(t, get, &body)
	if body.Status != "inactive" {
		t.Errorf("after delete, status = %q, want inactive", body.Status)
	}
}
