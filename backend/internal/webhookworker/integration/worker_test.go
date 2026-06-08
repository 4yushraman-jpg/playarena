package webhookworker_integration_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
	"github.com/4yushraman-jpg/playarena/internal/webhooks"
	"github.com/4yushraman-jpg/playarena/internal/webhookworker"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testSecretKeyB64 is a fixed 32-byte AES key (all-zero bytes, base64-encoded).
const testSecretKeyB64 = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

// testSecretKey is the raw 32-byte AES key derived from testSecretKeyB64.
var testSecretKey = make([]byte, 32) // all zeros

// testRawSecret is a fixed 32-byte plaintext webhook secret for test endpoints.
const testRawSecret = "test-webhook-secret-32-bytes-val"

func newWorker(t testing.TB, client *http.Client) *webhookworker.WebhookWorker {
	t.Helper()
	w, err := webhookworker.NewWebhookWorker(testPool, testSecretKeyB64, client, time.Minute, discardLog(), nil)
	if err != nil {
		t.Fatalf("NewWebhookWorker: %v", err)
	}
	return w
}

// createEndpoint inserts a webhook endpoint with a correctly-encrypted secret
// using the all-zero test key so the worker can decrypt and sign at delivery.
func createEndpoint(t testing.TB, ctx context.Context, orgID, userID pgtype.UUID, url string) pgtype.UUID {
	t.Helper()
	ciphertext, err := webhooks.EncryptSecret(testSecretKey, testRawSecret)
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}
	queries := db.New(testPool)
	ep, err := queries.CreateWebhookEndpoint(ctx, db.CreateWebhookEndpointParams{
		OrganizationID:   orgID,
		Url:              url,
		SecretCiphertext: ciphertext,
		Description:      nil,
		Active:           true,
		CreatedBy:        userID,
	})
	if err != nil {
		t.Fatalf("createEndpoint: %v", err)
	}
	return ep.ID
}

// seedDelivery inserts an outbox entry + webhook_deliveries row via SQL.
func seedDelivery(t testing.TB, ctx context.Context, orgID, endpointID pgtype.UUID) pgtype.UUID {
	t.Helper()
	var outboxID pgtype.UUID
	if err := testPool.QueryRow(ctx, `
		INSERT INTO notification_outbox (organization_id, event_type, entity_type, entity_id, payload)
		VALUES ($1, 'match_started', 'match', $1, '{"score":0}')
		RETURNING id
	`, orgID).Scan(&outboxID); err != nil {
		t.Fatalf("seedDelivery: insert outbox: %v", err)
	}
	var deliveryID pgtype.UUID
	if err := testPool.QueryRow(ctx, `
		INSERT INTO webhook_deliveries
		    (organization_id, endpoint_id, outbox_id, event_type, entity_type, entity_id, payload)
		VALUES ($1, $2, $3, 'match_started', 'match', $1, '{"score":0}')
		RETURNING id
	`, orgID, endpointID, outboxID).Scan(&deliveryID); err != nil {
		t.Fatalf("seedDelivery: insert delivery: %v", err)
	}
	return deliveryID
}

// captureHandler records incoming requests.
type captureHandler struct {
	received atomic.Int32
	bodies   chan []byte
	status   int
}

func newCapture(bufSize, status int) *captureHandler {
	return &captureHandler{bodies: make(chan []byte, bufSize), status: status}
}

func (h *captureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	select {
	case h.bodies <- body:
	default:
	}
	h.received.Add(1)
	w.WriteHeader(h.status)
}

// passthruClient returns an http.Client that uses the standard transport.
// Test servers use plain HTTP; the SSRF transport is bypassed via injection.
func passthruClient() *http.Client {
	return &http.Client{Transport: http.DefaultTransport}
}

// advanceLease moves lease_expires_at to the past so the row is claimable again.
func advanceLease(t testing.TB, ctx context.Context, deliveryID pgtype.UUID) {
	t.Helper()
	if _, err := testPool.Exec(ctx,
		"UPDATE webhook_deliveries SET lease_expires_at = NOW() - INTERVAL '1 second' WHERE id = $1",
		deliveryID,
	); err != nil {
		t.Fatalf("advanceLease: %v", err)
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestWebhookWorker_Deliver_Success verifies that a pending delivery is claimed,
// POSTed to the endpoint URL, and sent_at is set on success.
func TestWebhookWorker_Deliver_Success(t *testing.T) {
	ctx := context.Background()
	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	h := newCapture(10, http.StatusOK)
	srv := httptest.NewServer(h)
	defer srv.Close()

	ep := createEndpoint(t, ctx, org.ID, user.ID, srv.URL+"/hook")
	deliveryID := seedDelivery(t, ctx, org.ID, ep)

	worker := newWorker(t, passthruClient())
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	// Verify delivery received.
	select {
	case body := <-h.bodies:
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("invalid JSON payload: %v", err)
		}
		if payload["version"] != "1" {
			t.Errorf("payload.version = %v, want 1", payload["version"])
		}
		if payload["event_type"] != "match_started" {
			t.Errorf("payload.event_type = %v, want match_started", payload["event_type"])
		}
		// Signed headers must be present (validated by verifying body only — full
		// signature verification is in the webhook_test.go crypto round-trip test).
	case <-time.After(3 * time.Second):
		t.Fatal("no delivery received by test server after 3 s")
	}

	// sent_at must be set.
	var sentAt pgtype.Timestamptz
	if err := testPool.QueryRow(ctx,
		"SELECT sent_at FROM webhook_deliveries WHERE id = $1", deliveryID,
	).Scan(&sentAt); err != nil {
		t.Fatalf("select sent_at: %v", err)
	}
	if !sentAt.Valid {
		t.Error("sent_at is NULL after successful delivery, want non-NULL")
	}
}

// TestWebhookWorker_Deliver_Idempotent verifies that a second Drain does not
// re-deliver a row whose sent_at is already set.
func TestWebhookWorker_Deliver_Idempotent(t *testing.T) {
	ctx := context.Background()
	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	h := newCapture(10, http.StatusOK)
	srv := httptest.NewServer(h)
	defer srv.Close()

	ep := createEndpoint(t, ctx, org.ID, user.ID, srv.URL+"/hook")
	seedDelivery(t, ctx, org.ID, ep)

	worker := newWorker(t, passthruClient())
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("first Drain: %v", err)
	}
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("second Drain: %v", err)
	}

	if n := h.received.Load(); n != 1 {
		t.Errorf("received %d deliveries, want 1 (idempotent)", n)
	}
}

// TestWebhookWorker_SignatureVerification verifies the end-to-end HMAC-SHA256
// signing path.  The captured signature must match a locally-recomputed HMAC
// over the canonical string <timestamp>\n<event_id>\n<body>, and a tampered
// body must produce a different signature.  The test also asserts that
// payload.timestamp (RFC3339) and X-PlayArena-Timestamp (Unix seconds) refer
// to the same instant, catching any regression to separate time.Now() calls.
func TestWebhookWorker_SignatureVerification(t *testing.T) {
	ctx := context.Background()
	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	type captured struct {
		sig     string
		ts      string
		eventID string
		body    []byte
	}
	ch := make(chan captured, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ch <- captured{
			sig:     r.Header.Get("X-PlayArena-Signature"),
			ts:      r.Header.Get("X-PlayArena-Timestamp"),
			eventID: r.Header.Get("X-PlayArena-Event-ID"),
			body:    body,
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ep := createEndpoint(t, ctx, org.ID, user.ID, srv.URL+"/hook")
	deliveryID := seedDelivery(t, ctx, org.ID, ep)

	worker := newWorker(t, passthruClient())
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	var got captured
	select {
	case got = <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("no delivery received by test server after 3 s")
	}

	// All three signing headers must be present.
	if got.sig == "" {
		t.Fatal("X-PlayArena-Signature is empty")
	}
	if got.ts == "" {
		t.Fatal("X-PlayArena-Timestamp is empty")
	}
	wantEventID := pgutil.UUIDToString(deliveryID)
	if got.eventID != wantEventID {
		t.Errorf("X-PlayArena-Event-ID = %q, want %q (delivery UUID)", got.eventID, wantEventID)
	}

	// Recompute HMAC-SHA256 over canonical string: <timestamp>\n<event_id>\n<body>
	mac1 := hmac.New(sha256.New, []byte(testRawSecret))
	mac1.Write([]byte(got.ts + "\n" + got.eventID + "\n"))
	mac1.Write(got.body)
	wantSig := hex.EncodeToString(mac1.Sum(nil))

	if got.sig != wantSig {
		t.Errorf("signature mismatch\n  received %s\n  computed %s", got.sig, wantSig)
	}

	// Timestamp consistency: X-PlayArena-Timestamp (Unix) and payload.timestamp
	// (RFC3339) must represent the same second.  Divergence means two separate
	// time.Now() calls were used — a receiver using the body timestamp to verify
	// the signature would fail on every delivery.
	var parsed map[string]any
	if err := json.Unmarshal(got.body, &parsed); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	bodyTS, _ := parsed["timestamp"].(string)
	if bodyTS == "" {
		t.Fatal("payload.timestamp is missing from delivery body")
	}
	bodyTime, err := time.Parse(time.RFC3339, bodyTS)
	if err != nil {
		t.Fatalf("parse payload.timestamp %q: %v", bodyTS, err)
	}
	headerUnix, err := strconv.ParseInt(got.ts, 10, 64)
	if err != nil {
		t.Fatalf("parse X-PlayArena-Timestamp %q: %v", got.ts, err)
	}
	if bodyTime.Unix() != headerUnix {
		t.Errorf("timestamp divergence: payload.timestamp=%q (%d) != X-PlayArena-Timestamp=%d",
			bodyTS, bodyTime.Unix(), headerUnix)
	}

	// Mutation: a tampered body must produce a different signature.
	tampered := append(append([]byte(nil), got.body[:len(got.body)-1]...), []byte(`,"_tamper":1}`)...)
	mac2 := hmac.New(sha256.New, []byte(testRawSecret))
	mac2.Write([]byte(got.ts + "\n" + got.eventID + "\n"))
	mac2.Write(tampered)
	if hex.EncodeToString(mac2.Sum(nil)) == got.sig {
		t.Error("tampered body produced the same signature — HMAC is not protecting body integrity")
	}
}

// TestWebhookWorker_Retry_On5xx verifies that a 5xx causes a retry
// (sent_at NULL, failed_permanently false, attempt_count incremented).
func TestWebhookWorker_Retry_On5xx(t *testing.T) {
	ctx := context.Background()
	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	h := newCapture(10, http.StatusInternalServerError)
	srv := httptest.NewServer(h)
	defer srv.Close()

	ep := createEndpoint(t, ctx, org.ID, user.ID, srv.URL+"/hook")
	deliveryID := seedDelivery(t, ctx, org.ID, ep)

	worker := newWorker(t, passthruClient())
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	var sentAt pgtype.Timestamptz
	var perm bool
	var attempts int32
	if err := testPool.QueryRow(ctx,
		"SELECT sent_at, failed_permanently, attempt_count FROM webhook_deliveries WHERE id = $1",
		deliveryID,
	).Scan(&sentAt, &perm, &attempts); err != nil {
		t.Fatalf("select: %v", err)
	}
	if sentAt.Valid {
		t.Error("sent_at set after 5xx, want NULL")
	}
	if perm {
		t.Error("failed_permanently = TRUE after 1 attempt, want FALSE")
	}
	if attempts != 1 {
		t.Errorf("attempt_count = %d, want 1", attempts)
	}
}

// TestWebhookWorker_PermanentFailure_On4xx verifies that HTTP 4xx (non-429)
// causes immediate permanent failure in a single attempt.
func TestWebhookWorker_PermanentFailure_On4xx(t *testing.T) {
	ctx := context.Background()
	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	h := newCapture(10, http.StatusBadRequest)
	srv := httptest.NewServer(h)
	defer srv.Close()

	ep := createEndpoint(t, ctx, org.ID, user.ID, srv.URL+"/hook")
	deliveryID := seedDelivery(t, ctx, org.ID, ep)

	worker := newWorker(t, passthruClient())
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	var perm bool
	var attempts int32
	if err := testPool.QueryRow(ctx,
		"SELECT failed_permanently, attempt_count FROM webhook_deliveries WHERE id = $1",
		deliveryID,
	).Scan(&perm, &attempts); err != nil {
		t.Fatalf("select: %v", err)
	}
	if !perm {
		t.Errorf("failed_permanently = FALSE after 400, want TRUE (immediate dead-letter)")
	}
}

// TestWebhookWorker_HTTP429_Retry verifies that HTTP 429 (Too Many Requests)
// is treated as a retryable response, not an immediate permanent failure.
// 429 is the only 4xx that must NOT dead-letter on the first attempt.
func TestWebhookWorker_HTTP429_Retry(t *testing.T) {
	ctx := context.Background()
	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	h := newCapture(10, http.StatusTooManyRequests)
	srv := httptest.NewServer(h)
	defer srv.Close()

	ep := createEndpoint(t, ctx, org.ID, user.ID, srv.URL+"/hook")
	deliveryID := seedDelivery(t, ctx, org.ID, ep)

	worker := newWorker(t, passthruClient())
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	var sentAt pgtype.Timestamptz
	var perm bool
	var attempts int32
	if err := testPool.QueryRow(ctx,
		"SELECT sent_at, failed_permanently, attempt_count FROM webhook_deliveries WHERE id = $1",
		deliveryID,
	).Scan(&sentAt, &perm, &attempts); err != nil {
		t.Fatalf("select: %v", err)
	}
	if sentAt.Valid {
		t.Error("sent_at set after 429, want NULL (not a success)")
	}
	if perm {
		t.Error("failed_permanently = TRUE after 429, want FALSE (429 must retry, not dead-letter)")
	}
	if attempts != 1 {
		t.Errorf("attempt_count = %d after first 429, want 1", attempts)
	}
}

// TestWebhookWorker_DeadLetter_After3Attempts verifies that 3 consecutive 5xx
// failures mark the delivery permanently failed and stop retries.
func TestWebhookWorker_DeadLetter_After3Attempts(t *testing.T) {
	ctx := context.Background()
	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	h := newCapture(10, http.StatusServiceUnavailable)
	srv := httptest.NewServer(h)
	defer srv.Close()

	ep := createEndpoint(t, ctx, org.ID, user.ID, srv.URL+"/hook")
	deliveryID := seedDelivery(t, ctx, org.ID, ep)

	worker := newWorker(t, passthruClient())

	for i := 0; i < 3; i++ {
		if err := worker.Drain(ctx); err != nil {
			t.Fatalf("Drain %d: %v", i+1, err)
		}
		if i < 2 {
			advanceLease(t, ctx, deliveryID)
		}
	}

	var perm bool
	var attempts int32
	if err := testPool.QueryRow(ctx,
		"SELECT failed_permanently, attempt_count FROM webhook_deliveries WHERE id = $1",
		deliveryID,
	).Scan(&perm, &attempts); err != nil {
		t.Fatalf("select: %v", err)
	}
	if !perm {
		t.Errorf("failed_permanently = FALSE after %d attempts, want TRUE", attempts)
	}

	// 4th drain must not claim the dead-lettered row.
	advanceLease(t, ctx, deliveryID)
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("4th Drain: %v", err)
	}
	var finalAttempts int32
	if err := testPool.QueryRow(ctx,
		"SELECT attempt_count FROM webhook_deliveries WHERE id = $1", deliveryID,
	).Scan(&finalAttempts); err != nil {
		t.Fatalf("select attempt_count after 4th drain: %v", err)
	}
	if finalAttempts != 3 {
		t.Errorf("attempt_count = %d after dead-letter + 4th drain, want 3", finalAttempts)
	}
}

// TestWebhookWorker_ConcurrentWorkers verifies that two concurrent workers
// do not double-deliver the same row (FOR UPDATE SKIP LOCKED ensures this).
func TestWebhookWorker_ConcurrentWorkers(t *testing.T) {
	ctx := context.Background()
	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	h := newCapture(20, http.StatusOK)
	srv := httptest.NewServer(h)
	defer srv.Close()

	ep := createEndpoint(t, ctx, org.ID, user.ID, srv.URL+"/hook")
	seedDelivery(t, ctx, org.ID, ep)

	worker1 := newWorker(t, passthruClient())
	worker2 := newWorker(t, passthruClient())

	done := make(chan error, 2)
	go func() { done <- worker1.Drain(ctx) }()
	go func() { done <- worker2.Drain(ctx) }()

	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent Drain error: %v", err)
		}
	}

	// Both Drains have returned; deliver() is synchronous inside runOnce, so
	// all deliveries are complete at this point — no sleep required.
	if n := h.received.Load(); n != 1 {
		t.Errorf("concurrent workers delivered %d times, want 1 (SKIP LOCKED)", n)
	}
}

// TestWebhookWorker_StartStop exercises the Start/Stop/Drain lifecycle
// without panicking or deadlocking.
func TestWebhookWorker_StartStop(t *testing.T) {
	h := newCapture(1, http.StatusOK)
	srv := httptest.NewServer(h)
	defer srv.Close()

	worker := newWorker(t, passthruClient())
	worker.Start()
	time.Sleep(50 * time.Millisecond)
	worker.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := worker.Drain(ctx); err != nil {
		t.Errorf("Drain after Stop: %v", err)
	}
}

// TestWebhookWorker_TenantIsolation verifies that the worker correctly routes
// deliveries — org1's delivery goes to org1's endpoint, not org2's.
func TestWebhookWorker_TenantIsolation(t *testing.T) {
	ctx := context.Background()
	user1 := fixtures.CreateActiveUser(ctx, t, testPool)
	org1 := fixtures.CreateOrgForUser(ctx, t, testPool, user1.ID, "org_owner")
	user2 := fixtures.CreateActiveUser(ctx, t, testPool)
	org2 := fixtures.CreateOrgForUser(ctx, t, testPool, user2.ID, "org_owner")

	h1 := newCapture(10, http.StatusOK)
	srv1 := httptest.NewServer(h1)
	defer srv1.Close()

	h2 := newCapture(10, http.StatusOK)
	srv2 := httptest.NewServer(h2)
	defer srv2.Close()

	ep1 := createEndpoint(t, ctx, org1.ID, user1.ID, srv1.URL+"/hook")
	ep2 := createEndpoint(t, ctx, org2.ID, user2.ID, srv2.URL+"/hook")

	seedDelivery(t, ctx, org1.ID, ep1)
	seedDelivery(t, ctx, org2.ID, ep2)

	worker := newWorker(t, passthruClient())
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	if n := h1.received.Load(); n != 1 {
		t.Errorf("org1 server received %d deliveries, want 1", n)
	}
	if n := h2.received.Load(); n != 1 {
		t.Errorf("org2 server received %d deliveries, want 1", n)
	}
}
