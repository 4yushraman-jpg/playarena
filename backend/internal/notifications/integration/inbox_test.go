package notifications_integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── response structs ──────────────────────────────────────────────────────────

type notificationResponse struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	UserID         string  `json:"user_id"`
	EventType      string  `json:"event_type"`
	ReadAt         *string `json:"read_at"`
	CreatedAt      string  `json:"created_at"`
}

type notificationListResponse struct {
	Notifications []notificationResponse `json:"notifications"`
	Total         int64                  `json:"total"`
	Limit         int                    `json:"limit"`
	Offset        int                    `json:"offset"`
}

type errResp struct {
	Error string `json:"error"`
}

// ── token acquisition ─────────────────────────────────────────────────────────

func loginAs(t testing.TB, ts *testServer, emailAddr, password, orgID string) string {
	t.Helper()
	body := map[string]any{"email": emailAddr, "password": password}
	if orgID != "" {
		body["organization_id"] = orgID
	}
	resp := post(t, ts, "/api/v1/auth/login", body)
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
	var r struct {
		AccessToken string `json:"access_token"`
	}
	decodeBody(t, resp, &r)
	if r.AccessToken == "" {
		t.Fatal("loginAs: empty access_token")
	}
	return r.AccessToken
}

type orgContext struct {
	token   string
	orgID   string
	orgSlug string
}

func setupUserAndOrg(t testing.TB, ts *testServer, roleSlug string) orgContext {
	t.Helper()
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, ts.pool)
	org := fixtures.CreateOrgForUser(ctx, t, ts.pool, user.ID, roleSlug)
	orgIDStr := pgutil.UUIDToString(org.ID)

	token := loginAs(t, ts, user.Email, fixtures.KnownPasswordRaw, orgIDStr)
	return orgContext{token: token, orgID: orgIDStr, orgSlug: org.Slug}
}

// ── URL builders ──────────────────────────────────────────────────────────────

func notificationsURL(orgSlug string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/notifications", orgSlug)
}

func notificationURL(orgSlug, notifID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/notifications/%s", orgSlug, notifID)
}

func markReadURL(orgSlug, notifID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/notifications/%s/read", orgSlug, notifID)
}

func markAllReadURL(orgSlug string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/notifications/read-all", orgSlug)
}

func preferencesURL(orgSlug string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/notifications/preferences", orgSlug)
}

func preferenceURL(orgSlug, eventType string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/notifications/preferences/%s", orgSlug, eventType)
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func post(t testing.TB, ts *testServer, path string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func postWithHeaders(t testing.TB, ts *testServer, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func get(t testing.TB, ts *testServer, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.url+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func patch(t testing.TB, ts *testServer, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPatch, ts.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", path, err)
	}
	return resp
}

func put(t testing.TB, ts *testServer, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, ts.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", path, err)
	}
	return resp
}

func deleteReq(t testing.TB, ts *testServer, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, ts.url+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	return resp
}

// ── assertions ────────────────────────────────────────────────────────────────

func assertStatus(t testing.TB, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected HTTP %d, got %d; body: %s", want, resp.StatusCode, body)
	}
}

func decodeBody(t testing.TB, resp *http.Response, dest any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		t.Fatalf("decodeBody: %v", err)
	}
}

func bearerHeader(accessToken string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + accessToken}
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestNotification_List_Empty verifies GET /notifications returns an empty list
// when the user has no notifications.
func TestNotification_List_Empty(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := get(t, ts, notificationsURL(actor.orgSlug), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list notificationListResponse
	decodeBody(t, resp, &list)
	if list.Total != 0 {
		t.Errorf("total = %d, want 0 for fresh user", list.Total)
	}
}

// TestNotification_List_NoAuth verifies GET /notifications without a token returns 401.
func TestNotification_List_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := get(t, ts, notificationsURL(actor.orgSlug), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

// TestNotification_Preferences_GetEmpty verifies GET /preferences returns a valid
// (possibly empty) preferences list.
func TestNotification_Preferences_GetEmpty(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := get(t, ts, preferencesURL(actor.orgSlug), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
}

// TestNotification_Preferences_Upsert verifies PUT /preferences/{event_type} creates
// or updates a preference.
func TestNotification_Preferences_Upsert(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := put(t, ts, preferenceURL(actor.orgSlug, "registration_approved"), map[string]any{
		"channel": "in_app",
		"enabled": true,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
}

// TestNotification_Preferences_InvalidEventType verifies an unknown event_type
// returns 400.
func TestNotification_Preferences_InvalidEventType(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := put(t, ts, preferenceURL(actor.orgSlug, "not_a_real_event"), map[string]any{
		"channel": "in_app",
		"enabled": true,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestNotification_List_IsolatedToCallerUser verifies that notifications for
// User A do not appear in User B's inbox (same org).
func TestNotification_List_IsolatedToCallerUser(t *testing.T) {
	ts := buildTestServer(t, testPool)

	actorA := setupUserAndOrg(t, ts, "org_owner")
	actorB := setupUserAndOrg(t, ts, "org_owner")

	// B's inbox should not contain entries from A's org.
	resp := get(t, ts, notificationsURL(actorA.orgSlug), bearerHeader(actorB.token))
	defer resp.Body.Close()
	// Either 403 (BOLA) or 200 with empty list (different org).
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusOK {
		t.Errorf("expected 403 or 200, got %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusOK {
		var list notificationListResponse
		decodeBody(t, resp, &list)
		if list.Total > 0 {
			t.Errorf("user B should not see user A's notifications; got %d", list.Total)
		}
	}
}

// ── additional helpers ────────────────────────────────────────────────────────

func mustUUID(t testing.TB, s string) pgtype.UUID {
	t.Helper()
	uid, err := pgutil.ParseUUID(s)
	if err != nil {
		t.Fatalf("mustUUID %q: %v", s, err)
	}
	return uid
}

type registrationNotifResponse struct {
	ID           string  `json:"id"`
	TournamentID string  `json:"tournament_id"`
	TeamID       *string `json:"team_id"`
	Status       string  `json:"status"`
}

func tournamentRegistrationsURL(orgSlug, tournamentID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/tournaments/%s/registrations", orgSlug, tournamentID)
}

func tournamentRegistrationURL(orgSlug, tournamentID, registrationID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/tournaments/%s/registrations/%s", orgSlug, tournamentID, registrationID)
}

// ── Item 3: end-to-end notification delivery ─────────────────────────────────

// TestNotification_EndToEnd_RegistrationApproved verifies that approving a
// registration writes an outbox entry and DrainOutbox fans it out to org
// members who are not the actor. Specifically:
//   - actorB (viewer, same org) receives 1 registration_approved notification
//   - actorA (the approver/actor) is excluded from fan-out and receives 0
func TestNotification_EndToEnd_RegistrationApproved(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	actorA := setupUserAndOrg(t, ts, "org_owner")
	orgUID := mustUUID(t, actorA.orgID)

	actorBUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	fixtures.AddUserToOrg(ctx, t, ts.pool, orgUID, actorBUser.ID, "viewer")
	actorBToken := loginAs(t, ts, actorBUser.Email, fixtures.KnownPasswordRaw, actorA.orgID)

	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	team, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamID := pgutil.UUIDToString(team.ID)

	// Register team (pending).
	r1 := postWithHeaders(t, ts, tournamentRegistrationsURL(actorA.orgSlug, tournamentID),
		map[string]any{"team_id": teamID}, bearerHeader(actorA.token))
	assertStatus(t, r1, http.StatusCreated)
	var reg registrationNotifResponse
	decodeBody(t, r1, &reg)
	r1.Body.Close()

	// actorA approves → domain service writes outbox + DrainOutbox runs synchronously.
	r2 := patch(t, ts, tournamentRegistrationURL(actorA.orgSlug, tournamentID, reg.ID),
		map[string]any{"status": "approved"}, bearerHeader(actorA.token))
	assertStatus(t, r2, http.StatusOK)
	r2.Body.Close()

	// actorB should receive exactly 1 registration_approved notification.
	r3 := get(t, ts, notificationsURL(actorA.orgSlug), bearerHeader(actorBToken))
	defer r3.Body.Close()
	assertStatus(t, r3, http.StatusOK)
	var bList notificationListResponse
	decodeBody(t, r3, &bList)
	if bList.Total != 1 {
		t.Errorf("actorB total = %d, want 1", bList.Total)
	}
	if len(bList.Notifications) > 0 && bList.Notifications[0].EventType != "registration_approved" {
		t.Errorf("event_type = %q, want registration_approved", bList.Notifications[0].EventType)
	}

	// actorA (the approver/actor) must NOT receive a notification.
	r4 := get(t, ts, notificationsURL(actorA.orgSlug), bearerHeader(actorA.token))
	defer r4.Body.Close()
	assertStatus(t, r4, http.StatusOK)
	var aList notificationListResponse
	decodeBody(t, r4, &aList)
	if aList.Total != 0 {
		t.Errorf("actorA total = %d, want 0 (actor excluded from fan-out)", aList.Total)
	}
}

// ── Item 7: preference enforcement ───────────────────────────────────────────

// TestNotification_Preference_DisabledEventType_NoDelivery verifies that when a
// user opts out of a specific event type (registration_approved / in_app),
// DrainOutbox skips them even when they are an eligible org member.
func TestNotification_Preference_DisabledEventType_NoDelivery(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	actorA := setupUserAndOrg(t, ts, "org_owner")
	orgUID := mustUUID(t, actorA.orgID)

	actorBUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	fixtures.AddUserToOrg(ctx, t, ts.pool, orgUID, actorBUser.ID, "viewer")
	actorBToken := loginAs(t, ts, actorBUser.Email, fixtures.KnownPasswordRaw, actorA.orgID)

	// actorB opts out of registration_approved / in_app.
	rPref := put(t, ts, preferenceURL(actorA.orgSlug, "registration_approved"), map[string]any{
		"channel": "in_app",
		"enabled": false,
	}, bearerHeader(actorBToken))
	rPref.Body.Close()
	assertStatus(t, rPref, http.StatusOK)

	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	team, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamID := pgutil.UUIDToString(team.ID)

	// Register team.
	r1 := postWithHeaders(t, ts, tournamentRegistrationsURL(actorA.orgSlug, tournamentID),
		map[string]any{"team_id": teamID}, bearerHeader(actorA.token))
	assertStatus(t, r1, http.StatusCreated)
	var reg registrationNotifResponse
	decodeBody(t, r1, &reg)
	r1.Body.Close()

	// actorA approves → DrainOutbox runs.
	r2 := patch(t, ts, tournamentRegistrationURL(actorA.orgSlug, tournamentID, reg.ID),
		map[string]any{"status": "approved"}, bearerHeader(actorA.token))
	assertStatus(t, r2, http.StatusOK)
	r2.Body.Close()

	// actorB opted out → inbox must be empty.
	r3 := get(t, ts, notificationsURL(actorA.orgSlug), bearerHeader(actorBToken))
	defer r3.Body.Close()
	assertStatus(t, r3, http.StatusOK)
	var bList notificationListResponse
	decodeBody(t, r3, &bList)
	if bList.Total != 0 {
		t.Errorf("actorB total = %d, want 0 (opted out of registration_approved)", bList.Total)
	}
}

// ── Item 9: drain idempotency ─────────────────────────────────────────────────

// TestNotification_Drain_Idempotency verifies that running DrainOutbox twice on
// the same entry (simulating a crash between drain and mark-processed) produces
// exactly 1 notification, not 2. This exercises the UNIQUE constraint
// (outbox_id, user_id, channel) + ON CONFLICT DO NOTHING on the notifications
// table.
func TestNotification_Drain_Idempotency(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	actorA := setupUserAndOrg(t, ts, "org_owner")
	orgUID := mustUUID(t, actorA.orgID)

	// actorB is a different org member so DrainOutbox has a fan-out recipient.
	actorBUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	fixtures.AddUserToOrg(ctx, t, ts.pool, orgUID, actorBUser.ID, "viewer")

	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	team, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamID := pgutil.UUIDToString(team.ID)

	// Register and approve to produce a real outbox entry + first drain.
	r1 := postWithHeaders(t, ts, tournamentRegistrationsURL(actorA.orgSlug, tournamentID),
		map[string]any{"team_id": teamID}, bearerHeader(actorA.token))
	assertStatus(t, r1, http.StatusCreated)
	var reg registrationNotifResponse
	decodeBody(t, r1, &reg)
	r1.Body.Close()

	r2 := patch(t, ts, tournamentRegistrationURL(actorA.orgSlug, tournamentID, reg.ID),
		map[string]any{"status": "approved"}, bearerHeader(actorA.token))
	assertStatus(t, r2, http.StatusOK)
	r2.Body.Close()

	// Simulate a crash: reset processed_at = NULL on the entry DrainOutbox just marked.
	// This causes the entry to appear pending again on the next drain call.
	if _, err := ts.pool.Exec(ctx, `
		UPDATE notification_outbox
		SET    processed_at = NULL
		WHERE  id = (
			SELECT id FROM notification_outbox
			WHERE  organization_id = $1
			  AND  processed_at IS NOT NULL
			ORDER  BY created_at DESC
			LIMIT  1
		)
	`, orgUID); err != nil {
		t.Fatalf("reset processed_at: %v", err)
	}

	// Second drain — must not create duplicate notifications.
	ts.notifSvc.DrainOutbox(ctx, orgUID, logger)

	// Total in_app notifications for this org must be exactly 1, not 2.
	var count int64
	if err := ts.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM notifications WHERE organization_id = $1 AND deleted_at IS NULL",
		orgUID).Scan(&count); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if count != 1 {
		t.Errorf("notification count = %d, want 1 (idempotent drain)", count)
	}
}

// ── D4-10: notification CRUD endpoints ───────────────────────────────────────

// seedOrgNotification inserts a system outbox entry and drains it, placing one
// notification in every org member's inbox. Returns the notification ID for the
// given actor.
func seedOrgNotification(t testing.TB, ts *testServer, orgUID pgtype.UUID, orgSlug, token string) string {
	t.Helper()
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// System outbox entry (NULL actor → all members receive the notification).
	if _, err := ts.pool.Exec(ctx, `
		INSERT INTO notification_outbox (organization_id, event_type, entity_type, entity_id, payload)
		VALUES ($1, 'registration_approved', 'tournament_registrations', $1, '{}')
	`, orgUID); err != nil {
		t.Fatalf("seedOrgNotification insert outbox: %v", err)
	}
	ts.notifSvc.DrainOutbox(ctx, orgUID, logger)

	resp := get(t, ts, notificationsURL(orgSlug), bearerHeader(token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var list notificationListResponse
	decodeBody(t, resp, &list)
	if list.Total < 1 || len(list.Notifications) < 1 {
		t.Fatal("seedOrgNotification: expected at least 1 notification after drain")
	}
	return list.Notifications[0].ID
}

// TestNotification_MarkRead_Success verifies that PATCH /{id}/read returns 200
// and sets read_at on the notification.
func TestNotification_MarkRead_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	orgUID := mustUUID(t, actor.orgID)

	notifID := seedOrgNotification(t, ts, orgUID, actor.orgSlug, actor.token)

	resp := patch(t, ts, markReadURL(actor.orgSlug, notifID), nil, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var n notificationResponse
	decodeBody(t, resp, &n)
	if n.ReadAt == nil {
		t.Error("read_at should be set after marking read")
	}
}

// TestNotification_MarkAllRead_Success verifies that POST /read-all returns 204
// and every notification in the inbox has read_at set afterward.
func TestNotification_MarkAllRead_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	orgUID := mustUUID(t, actor.orgID)

	// Seed two notifications.
	seedOrgNotification(t, ts, orgUID, actor.orgSlug, actor.token)
	seedOrgNotification(t, ts, orgUID, actor.orgSlug, actor.token)

	resp := postWithHeaders(t, ts, markAllReadURL(actor.orgSlug), nil, bearerHeader(actor.token))
	resp.Body.Close()
	assertStatus(t, resp, http.StatusNoContent)

	// Verify all notifications now have read_at set.
	listResp := get(t, ts, notificationsURL(actor.orgSlug), bearerHeader(actor.token))
	defer listResp.Body.Close()
	assertStatus(t, listResp, http.StatusOK)
	var list notificationListResponse
	decodeBody(t, listResp, &list)
	if list.Total < 2 {
		t.Fatalf("expected at least 2 notifications, got %d", list.Total)
	}
	for _, n := range list.Notifications {
		if n.ReadAt == nil {
			t.Errorf("notification %s still unread after read-all", n.ID)
		}
	}
}

// TestNotification_Delete_Success verifies that DELETE /{id} returns 204 and
// subsequent GET /{id} returns 404 (soft-deleted notification not found).
func TestNotification_Delete_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")
	orgUID := mustUUID(t, actor.orgID)

	notifID := seedOrgNotification(t, ts, orgUID, actor.orgSlug, actor.token)

	delResp := deleteReq(t, ts, notificationURL(actor.orgSlug, notifID), bearerHeader(actor.token))
	delResp.Body.Close()
	assertStatus(t, delResp, http.StatusNoContent)

	getResp := get(t, ts, notificationURL(actor.orgSlug, notifID), bearerHeader(actor.token))
	defer getResp.Body.Close()
	assertStatus(t, getResp, http.StatusNotFound)
}
