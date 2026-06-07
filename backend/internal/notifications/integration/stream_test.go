package notifications_integration_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// streamURL returns the SSE stream endpoint path for an org.
func streamURL(orgSlug string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/notifications/stream", orgSlug)
}

// connectSSE opens an SSE stream using the ?token= query param.
// Returns the open response and a cancel function to disconnect.
// The caller must call cancel() and resp.Body.Close() when done.
func connectSSE(t testing.TB, ts *testServer, orgSlug, token string) (*http.Response, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	url := ts.url + streamURL(orgSlug) + "?token=" + token
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		t.Fatalf("connectSSE: build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("connectSSE: do request: %v", err)
	}
	return resp, cancel
}

// readSSELines pumps SSE response body lines into a buffered channel.
// The goroutine exits when the body is closed or reaches EOF.
func readSSELines(body io.Reader) <-chan string {
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(body)
		for scanner.Scan() {
			ch <- scanner.Text()
		}
	}()
	return ch
}

// waitLine reads from lines until one with the given prefix arrives,
// or the timeout expires (fatal failure).
func waitLine(t testing.TB, lines <-chan string, prefix string, timeout time.Duration) string {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatalf("waitLine: SSE channel closed while waiting for prefix %q", prefix)
			}
			if strings.HasPrefix(line, prefix) {
				return line
			}
		case <-deadline.C:
			t.Fatalf("waitLine: timeout waiting for SSE line with prefix %q", prefix)
			return ""
		}
	}
}

// drainKeepalive reads the initial ":\n\n" keepalive frame from the SSE body.
func drainKeepalive(t testing.TB, body io.Reader) {
	t.Helper()
	buf := make([]byte, 3)
	if _, err := io.ReadFull(body, buf); err != nil {
		t.Fatalf("drainKeepalive: %v", err)
	}
	if string(buf) != ":\n\n" {
		t.Errorf("expected keepalive %q, got %q", ":\n\n", string(buf))
	}
}

// ── auth / tenant-isolation tests ────────────────────────────────────────────

// TestStream_NoToken_Unauthorized verifies GET /stream without a token returns 401.
func TestStream_NoToken_Unauthorized(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := get(t, ts, streamURL(actor.orgSlug), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

// TestStream_InvalidToken_Unauthorized verifies a malformed token returns 401.
func TestStream_InvalidToken_Unauthorized(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	url := ts.url + streamURL(actor.orgSlug) + "?token=not.a.valid.jwt"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

// TestStream_WrongOrg_Forbidden verifies that a valid token for org A cannot
// subscribe to org B's stream (tenant isolation).
func TestStream_WrongOrg_Forbidden(t *testing.T) {
	ts := buildTestServer(t, testPool)

	actorA := setupUserAndOrg(t, ts, "org_owner")
	actorB := setupUserAndOrg(t, ts, "org_owner")

	// actorA's JWT has OrganizationID = orgA; orgB.slug is a different org.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	url := ts.url + streamURL(actorB.orgSlug) + "?token=" + actorA.token
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// ── connection / protocol tests ───────────────────────────────────────────────

// TestStream_Connect_ContentType verifies a valid connection returns
// Content-Type: text/event-stream.
func TestStream_Connect_ContentType(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp, cancel := connectSSE(t, ts, actor.orgSlug, actor.token)
	defer cancel()
	defer resp.Body.Close()

	assertStatus(t, resp, http.StatusOK)
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}

// TestStream_Connect_ReceivesKeepalive verifies the server sends ":\n\n"
// immediately on connect, before any events are published.
func TestStream_Connect_ReceivesKeepalive(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp, cancel := connectSSE(t, ts, actor.orgSlug, actor.token)
	defer cancel()
	defer resp.Body.Close()

	assertStatus(t, resp, http.StatusOK)
	drainKeepalive(t, resp.Body) // asserts content and fails on error
}

// TestStream_BearerHeaderAuth verifies that Authorization: Bearer <token> also
// authenticates the SSE stream (fallback for curl / server-side clients).
func TestStream_BearerHeaderAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	url := ts.url + streamURL(actor.orgSlug)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+actor.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
}

// ── end-to-end publish tests ──────────────────────────────────────────────────

// TestStream_DrainOutbox_DeliversSseEvent verifies the full publish path:
//  1. actorB connects to the SSE stream (before the event).
//  2. A system outbox entry is inserted and DrainOutbox is called.
//  3. actorB's connected client receives an "event: notification" frame.
//  4. The data payload decodes to a valid notification with the expected event_type.
func TestStream_DrainOutbox_DeliversSseEvent(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	actorA := setupUserAndOrg(t, ts, "org_owner")
	orgUID := mustUUID(t, actorA.orgID)

	actorBUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	fixtures.AddUserToOrg(ctx, t, ts.pool, orgUID, actorBUser.ID, "viewer")
	actorBToken := loginAs(t, ts, actorBUser.Email, fixtures.KnownPasswordRaw, actorA.orgID)

	// Open actorB's SSE stream before the event is published.
	resp, cancelStream := connectSSE(t, ts, actorA.orgSlug, actorBToken)
	defer cancelStream()
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	drainKeepalive(t, resp.Body)

	lines := readSSELines(resp.Body)

	// Insert a system outbox entry (NULL actor → all org members receive).
	if _, err := ts.pool.Exec(ctx, `
		INSERT INTO notification_outbox (organization_id, event_type, entity_type, entity_id, payload)
		VALUES ($1, 'registration_approved', 'tournament_registrations', $1, '{}')
	`, orgUID); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ts.notifSvc.DrainOutbox(ctx, orgUID, logger)

	eventLine := waitLine(t, lines, "event:", 5*time.Second)
	if eventLine != "event: notification" {
		t.Errorf("event line = %q, want %q", eventLine, "event: notification")
	}

	dataLine := waitLine(t, lines, "data:", 5*time.Second)
	payload := strings.TrimPrefix(dataLine, "data: ")

	var n notificationResponse
	if err := json.Unmarshal([]byte(payload), &n); err != nil {
		t.Fatalf("data payload is not valid notification JSON: %v\npayload: %s", err, payload)
	}
	if n.EventType != "registration_approved" {
		t.Errorf("event_type = %q, want registration_approved", n.EventType)
	}
	if n.UserID != pgutil.UUIDToString(actorBUser.ID) {
		t.Errorf("user_id = %q, want actorB %q", n.UserID, pgutil.UUIDToString(actorBUser.ID))
	}
}

// TestStream_ActorExcluded_NoSseEvent verifies that the actor who triggered an
// event does not receive an SSE notification for it (fan-out excludes the actor).
func TestStream_ActorExcluded_NoSseEvent(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create actorA explicitly so we have their user ID for the actor_id field.
	actorAUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	org := fixtures.CreateOrgForUser(ctx, t, ts.pool, actorAUser.ID, "org_owner")
	orgUID := org.ID
	orgIDStr := pgutil.UUIDToString(org.ID)
	actorAToken := loginAs(t, ts, actorAUser.Email, fixtures.KnownPasswordRaw, orgIDStr)

	// Connect actorA to their SSE stream.
	resp, cancelStream := connectSSE(t, ts, org.Slug, actorAToken)
	defer cancelStream()
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	drainKeepalive(t, resp.Body)

	// Insert an outbox entry with actorA as the actor.
	// DrainOutbox excludes the actor, so actorA receives no notification.
	actorAUID := pgtype.UUID{Bytes: actorAUser.ID.Bytes, Valid: true}
	if _, err := ts.pool.Exec(ctx, `
		INSERT INTO notification_outbox
			(organization_id, event_type, actor_id, entity_type, entity_id, payload)
		VALUES ($1, 'registration_approved', $2, 'tournament_registrations', $1, '{}')
	`, orgUID, actorAUID); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	ts.notifSvc.DrainOutbox(ctx, orgUID, logger)

	// No SSE event should arrive for actorA within 500ms.
	lines := readSSELines(resp.Body)
	select {
	case line := <-lines:
		if strings.HasPrefix(line, "event:") {
			t.Errorf("actorA received SSE event %q — actor should be excluded from fan-out", line)
		}
	case <-time.After(500 * time.Millisecond):
		// Correct — no event.
	}
}

// TestStream_HubShutdown_DisconnectsClients verifies that Hub.Shutdown() causes
// all connected SSE clients to disconnect (their response bodies reach EOF).
func TestStream_HubShutdown_DisconnectsClients(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp, cancelStream := connectSSE(t, ts, actor.orgSlug, actor.token)
	defer cancelStream()

	assertStatus(t, resp, http.StatusOK)
	drainKeepalive(t, resp.Body)

	// Shut down the hub — subscriber channels are closed → SSE handler exits.
	ts.hub.Shutdown()

	// The response body should reach EOF shortly after shutdown.
	done := make(chan struct{})
	go func() {
		defer close(done)
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
	}()

	select {
	case <-done:
		// Connection closed — correct.
	case <-time.After(3 * time.Second):
		t.Error("SSE connection not closed within 3s after Hub.Shutdown()")
	}
}

// ── additional coverage: expired token, cross-user isolation, concurrent clients,
//    disconnect cleanup, drain idempotency ─────────────────────────────────────

// makeExpiredStreamToken builds a valid HS256 JWT whose exp is one hour in the
// past. Used to verify that the SSE handler rejects stale tokens with 401.
func makeExpiredStreamToken(t testing.TB, jwtSecret, userID, orgID string) string {
	t.Helper()
	now := time.Now()
	claims := auth.JWTClaims{
		UserID:         userID,
		OrganizationID: orgID,
		Role:           "viewer",
		Email:          "expired-test@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "playarena",
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			NotBefore: jwt.NewNumericDate(now.Add(-2 * time.Hour)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(jwtSecret))
	if err != nil {
		t.Fatalf("makeExpiredStreamToken: %v", err)
	}
	return signed
}

// TestStream_ExpiredToken_Unauthorized verifies that a correctly signed JWT whose
// exp claim is in the past returns 401 (not 200 or 500).
func TestStream_ExpiredToken_Unauthorized(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	// Use a dummy UUID for user_id — token is rejected before any DB lookup.
	expiredToken := makeExpiredStreamToken(t, ts.cfg.JWTSecret,
		"00000000-0000-0000-0000-000000000099", actor.orgID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	url := ts.url + streamURL(actor.orgSlug) + "?token=" + expiredToken
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

// TestStream_CrossUserIsolation verifies that a notification published to userB's
// hub slot never reaches userA's connected SSE stream, even within the same org.
// This is a direct regression gate for the Hub's (orgID, userID) key routing.
func TestStream_CrossUserIsolation(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	actorAUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	org := fixtures.CreateOrgForUser(ctx, t, ts.pool, actorAUser.ID, "org_owner")
	orgIDStr := pgutil.UUIDToString(org.ID)
	actorAToken := loginAs(t, ts, actorAUser.Email, fixtures.KnownPasswordRaw, orgIDStr)

	actorBUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	fixtures.AddUserToOrg(ctx, t, ts.pool, org.ID, actorBUser.ID, "viewer")

	// Only actorA subscribes to the SSE stream.
	resp, cancelStream := connectSSE(t, ts, org.Slug, actorAToken)
	defer cancelStream()
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	drainKeepalive(t, resp.Body)

	// Publish directly to actorB's hub slot — actorA's subscription key differs.
	ts.hub.Publish(org.ID, actorBUser.ID, map[string]string{"recipient": "actorB_only"})

	lines := readSSELines(resp.Body)
	select {
	case line := <-lines:
		if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:") {
			t.Errorf("actorA received message destined for actorB: %q", line)
		}
	case <-time.After(300 * time.Millisecond):
		// Correct — actorA received nothing.
	}
}

// TestStream_MultipleConcurrentClients verifies that two simultaneous SSE
// connections for the same (orgID, userID) both receive the published event.
// Models a "same user, two browser tabs" scenario — the Hub must fan out to
// all channels in the inner subscriber map, not just the first.
func TestStream_MultipleConcurrentClients(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	actorUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	org := fixtures.CreateOrgForUser(ctx, t, ts.pool, actorUser.ID, "org_owner")
	orgIDStr := pgutil.UUIDToString(org.ID)
	token := loginAs(t, ts, actorUser.Email, fixtures.KnownPasswordRaw, orgIDStr)

	resp1, cancel1 := connectSSE(t, ts, org.Slug, token)
	defer cancel1()
	defer resp1.Body.Close()
	assertStatus(t, resp1, http.StatusOK)
	drainKeepalive(t, resp1.Body)

	resp2, cancel2 := connectSSE(t, ts, org.Slug, token)
	defer cancel2()
	defer resp2.Body.Close()
	assertStatus(t, resp2, http.StatusOK)
	drainKeepalive(t, resp2.Body)

	lines1 := readSSELines(resp1.Body)
	lines2 := readSSELines(resp2.Body)

	if _, err := ts.pool.Exec(ctx, `
		INSERT INTO notification_outbox (organization_id, event_type, entity_type, entity_id, payload)
		VALUES ($1, 'match_created', 'matches', $1, '{}')
	`, org.ID); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	ts.notifSvc.DrainOutbox(ctx, org.ID, logger)

	// Both connections must independently receive the event.
	waitLine(t, lines1, "event:", 5*time.Second)
	waitLine(t, lines2, "event:", 5*time.Second)
}

// TestStream_ClientDisconnect_Cleanup verifies that after a client disconnects the
// server-side SSE goroutine cleans up its hub subscription, and a subsequent
// connection for the same user receives new events without interference.
func TestStream_ClientDisconnect_Cleanup(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	actorUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	org := fixtures.CreateOrgForUser(ctx, t, ts.pool, actorUser.ID, "org_owner")
	orgIDStr := pgutil.UUIDToString(org.ID)
	token := loginAs(t, ts, actorUser.Email, fixtures.KnownPasswordRaw, orgIDStr)

	// Connect then immediately disconnect.
	resp1, cancel1 := connectSSE(t, ts, org.Slug, token)
	assertStatus(t, resp1, http.StatusOK)
	drainKeepalive(t, resp1.Body)
	cancel1()
	resp1.Body.Close()

	// Allow the server-side goroutine to detect the cancellation and call
	// hub.Unsubscribe before reconnecting.
	time.Sleep(100 * time.Millisecond)

	// Reconnect — must work cleanly after the old subscription is gone.
	resp2, cancel2 := connectSSE(t, ts, org.Slug, token)
	defer cancel2()
	defer resp2.Body.Close()
	assertStatus(t, resp2, http.StatusOK)
	drainKeepalive(t, resp2.Body)

	lines2 := readSSELines(resp2.Body)

	if _, err := ts.pool.Exec(ctx, `
		INSERT INTO notification_outbox (organization_id, event_type, entity_type, entity_id, payload)
		VALUES ($1, 'match_started', 'matches', $1, '{}')
	`, org.ID); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
	ts.notifSvc.DrainOutbox(ctx, org.ID, logger)

	waitLine(t, lines2, "event:", 5*time.Second)
}

// TestStream_DrainOutbox_Idempotent_NoDuplicateSSE verifies that calling
// DrainOutbox twice for the same outbox entry produces exactly one SSE event.
// The second call finds no pending rows (UNIQUE constraint on outbox_id prevents
// re-processing) and publishes nothing.
//
// The assertion fully consumes the first SSE frame (event line + data line +
// blank-line terminator) before opening the 300ms duplicate window. Without
// this, the buffered data line from the first frame would cause the select to
// exit immediately, allowing a duplicate event hiding behind it to go undetected.
func TestStream_DrainOutbox_Idempotent_NoDuplicateSSE(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	actor := setupUserAndOrg(t, ts, "org_owner")
	orgUID := mustUUID(t, actor.orgID)

	resp, cancelStream := connectSSE(t, ts, actor.orgSlug, actor.token)
	defer cancelStream()
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	drainKeepalive(t, resp.Body)

	lines := readSSELines(resp.Body)

	if _, err := ts.pool.Exec(ctx, `
		INSERT INTO notification_outbox (organization_id, event_type, entity_type, entity_id, payload)
		VALUES ($1, 'match_completed', 'matches', $1, '{}')
	`, orgUID); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}

	// First drain: creates the notification row and publishes one SSE event.
	ts.notifSvc.DrainOutbox(ctx, orgUID, logger)
	// Second drain: outbox entry already processed — no new rows, no publish.
	ts.notifSvc.DrainOutbox(ctx, orgUID, logger)

	// Consume the event line of the first (and only) frame.
	eventLine := waitLine(t, lines, "event:", 5*time.Second)
	if eventLine != "event: notification" {
		t.Errorf("event line = %q, want %q", eventLine, "event: notification")
	}
	// Consume the data payload line of the first frame.
	waitLine(t, lines, "data:", 5*time.Second)
	// Consume the blank-line terminator (\n\n) of the first frame so it cannot
	// preempt the duplicate-event check below.
	select {
	case <-lines: // blank separator — discard
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SSE frame blank-line terminator")
	}

	// With the entire first frame consumed, verify no second event arrives.
	select {
	case line := <-lines:
		if strings.HasPrefix(line, "event:") {
			t.Errorf("duplicate SSE event %q — DrainOutbox idempotency broken", line)
		}
	case <-time.After(300 * time.Millisecond):
		// Correct — exactly one event delivered.
	}
}

// ── MT-1: platform admin token isolation ─────────────────────────────────────

// makePlatformAdminToken builds a valid HS256 JWT whose OrganizationID is empty.
// Platform admins have no org scope. The SSE handler must reject them with 403
// because SSE subscriptions are always org-scoped.
func makePlatformAdminToken(t testing.TB, jwtSecret string) string {
	t.Helper()
	now := time.Now()
	const platformUserID = "00000000-0000-0000-0000-000000000099"
	claims := auth.JWTClaims{
		UserID:         platformUserID,
		OrganizationID: "", // empty — the platform-admin indicator
		Role:           "platform_admin",
		Email:          "platform-admin-test@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "playarena",
			Subject:   platformUserID,
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(jwtSecret))
	if err != nil {
		t.Fatalf("makePlatformAdminToken: %v", err)
	}
	return signed
}

// TestStream_PlatformAdminToken_Forbidden verifies that a valid platform-admin
// JWT (OrganizationID == "") is rejected with 403 when attempting to subscribe
// to an org-scoped SSE stream.
//
// Regression gate: removing or bypassing the org-comparison check at
//
//	handler.go: claims.OrganizationID != pgutil.UUIDToString(org.ID)
//
// would cause the platform admin to receive 200. The test would then fail,
// catching the regression before it reaches production.
func TestStream_PlatformAdminToken_Forbidden(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	platformToken := makePlatformAdminToken(t, ts.cfg.JWTSecret)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	url := ts.url + streamURL(actor.orgSlug) + "?token=" + platformToken
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// ── MT-3: subscribe-after-shutdown immediate disconnect ───────────────────────

// TestStream_SubscribeAfterShutdown_ImmediateDisconnect verifies that an SSE
// connection established after Hub.Shutdown() exits immediately.
//
// After Shutdown, h.hub.Done() is a closed channel. The SSE handler's select
// fires it on the first iteration and returns, so the client sees EOF shortly
// after receiving the initial keepalive.
//
// Regression gate: removing "case <-h.hub.Done(): return" from the SSE handler
// loop would leave the connection open indefinitely (the ticker keeps sending
// keepalives). The test times out at 2s and fails, catching the regression.
func TestStream_SubscribeAfterShutdown_ImmediateDisconnect(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	// Shut down the hub while the HTTP server is still running so we can
	// establish a new SSE connection to a dead hub.
	ts.hub.Shutdown()

	resp, cancelStream := connectSSE(t, ts, actor.orgSlug, actor.token)
	defer cancelStream()
	defer resp.Body.Close()

	// The handler writes SSE headers and the initial keepalive before entering
	// the select loop, so the connection is 200 even after hub shutdown.
	assertStatus(t, resp, http.StatusOK)
	drainKeepalive(t, resp.Body)

	// hub.Done() is already closed — the handler exits on the first select
	// iteration. The HTTP server terminates the response, client reads EOF.
	done := make(chan struct{})
	go func() {
		defer close(done)
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
	}()

	select {
	case <-done:
		// Correct — connection closed immediately after hub shutdown.
	case <-time.After(2 * time.Second):
		t.Error("SSE connection still open 2s after hub shutdown — case <-h.hub.Done() may be missing from the handler loop")
	}
}
