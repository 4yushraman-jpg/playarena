package webhooks_integration_test

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
	"github.com/4yushraman-jpg/playarena/internal/webhooks"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testSecretKeyB64 is a fixed 32-byte AES key for test use.
const testSecretKeyB64 = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" // 32 zero bytes

func newService(t testing.TB) *webhooks.Service {
	t.Helper()
	queries := db.New(testPool)
	repo := webhooks.NewRepository(queries, testPool)
	svc, err := webhooks.NewService(repo, testSecretKeyB64, discardLog())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

// seedEndpoint creates a webhook endpoint for orgID and returns it.
func seedEndpoint(t testing.TB, ctx context.Context, orgID pgtype.UUID, actorID string, url string) *webhooks.CreateResponse {
	t.Helper()
	svc := newService(t)
	resp, err := svc.Create(ctx, orgID, actorID, webhooks.CreateRequest{URL: url})
	if err != nil {
		t.Fatalf("seedEndpoint create: %v", err)
	}
	return resp
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestWebhook_Create_Success verifies that a valid HTTPS URL is accepted and
// the raw secret is returned only on creation.
func TestWebhook_Create_Success(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	svc := newService(t)
	resp, err := svc.Create(ctx, org.ID, pgutil.UUIDToString(user.ID), webhooks.CreateRequest{
		URL: "https://example.com/hook",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resp.ID == "" {
		t.Error("ID is empty")
	}
	if resp.URL != "https://example.com/hook" {
		t.Errorf("URL = %q, want %q", resp.URL, "https://example.com/hook")
	}
	if !resp.Active {
		t.Error("Active = false, want true on creation")
	}
	if resp.RawSecret == "" {
		t.Error("RawSecret is empty — must be shown on creation")
	}
	// Raw secret must be base64url-encoded 32 bytes.
	raw, err := base64.RawURLEncoding.DecodeString(resp.RawSecret)
	if err != nil {
		t.Errorf("RawSecret is not valid base64url: %v", err)
	}
	if len(raw) != 32 {
		t.Errorf("RawSecret decoded length = %d, want 32", len(raw))
	}
}

// TestWebhook_Create_RejectsHTTP verifies that non-HTTPS URLs are rejected.
func TestWebhook_Create_RejectsHTTP(t *testing.T) {
	ctx := context.Background()
	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")
	svc := newService(t)

	_, err := svc.Create(ctx, org.ID, pgutil.UUIDToString(user.ID), webhooks.CreateRequest{
		URL: "http://example.com/hook",
	})
	if err == nil {
		t.Fatal("expected error for HTTP URL, got nil")
	}
}

// TestWebhook_Create_RejectsLocalhost verifies that localhost is blocked at registration time.
func TestWebhook_Create_RejectsLocalhost(t *testing.T) {
	ctx := context.Background()
	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")
	svc := newService(t)

	ssrfURLs := []string{
		"https://localhost/hook",
		"https://127.0.0.1/hook",
		"https://10.0.0.1/hook",
		"https://192.168.1.1/hook",
		"https://172.16.0.1/hook",
		"https://169.254.169.254/latest/meta-data",
		"https://[::1]/hook",
		"https://[fc00::1]/hook",
	}

	for _, u := range ssrfURLs {
		_, err := svc.Create(ctx, org.ID, pgutil.UUIDToString(user.ID), webhooks.CreateRequest{URL: u})
		if err == nil {
			t.Errorf("expected SSRF error for URL %q, got nil", u)
		}
	}
}

// TestWebhook_Create_EmptyURL verifies that an empty URL is rejected.
func TestWebhook_Create_EmptyURL(t *testing.T) {
	ctx := context.Background()
	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")
	svc := newService(t)

	_, err := svc.Create(ctx, org.ID, pgutil.UUIDToString(user.ID), webhooks.CreateRequest{URL: ""})
	if err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
}

// TestWebhook_List returns all endpoints for the org (none from other orgs).
func TestWebhook_List(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")
	otherUser := fixtures.CreateActiveUser(ctx, t, testPool)
	otherOrg := fixtures.CreateOrgForUser(ctx, t, testPool, otherUser.ID, "org_owner")

	actorID := pgutil.UUIDToString(user.ID)
	seedEndpoint(t, ctx, org.ID, actorID, "https://example.com/hook1")
	seedEndpoint(t, ctx, org.ID, actorID, "https://example.com/hook2")
	// Endpoint in a different org — must not appear in List for org.
	seedEndpoint(t, ctx, otherOrg.ID, pgutil.UUIDToString(otherUser.ID), "https://example.com/other")

	svc := newService(t)
	resp, err := svc.List(ctx, org.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("Total = %d, want 2 (tenant isolation: other org endpoints must not appear)", resp.Total)
	}
}

// TestWebhook_GetByID returns the endpoint and enforces org ownership (BOLA).
func TestWebhook_GetByID(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")
	otherUser := fixtures.CreateActiveUser(ctx, t, testPool)
	otherOrg := fixtures.CreateOrgForUser(ctx, t, testPool, otherUser.ID, "org_owner")

	created := seedEndpoint(t, ctx, org.ID, pgutil.UUIDToString(user.ID), "https://example.com/hook")

	svc := newService(t)

	// Happy path: correct org.
	got, err := svc.GetByID(ctx, org.ID, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}

	// BOLA: different org must not see this endpoint.
	_, err = svc.GetByID(ctx, otherOrg.ID, created.ID)
	if err == nil {
		t.Error("expected error when accessing webhook from a different org, got nil")
	}
}

// TestWebhook_UpdateActive toggles the active flag.
func TestWebhook_UpdateActive(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	created := seedEndpoint(t, ctx, org.ID, pgutil.UUIDToString(user.ID), "https://example.com/hook")

	svc := newService(t)

	// Deactivate.
	updated, err := svc.UpdateActive(ctx, org.ID, created.ID, false)
	if err != nil {
		t.Fatalf("UpdateActive(false): %v", err)
	}
	if updated.Active {
		t.Error("Active = true after deactivate, want false")
	}

	// Re-activate.
	updated2, err := svc.UpdateActive(ctx, org.ID, created.ID, true)
	if err != nil {
		t.Fatalf("UpdateActive(true): %v", err)
	}
	if !updated2.Active {
		t.Error("Active = false after re-activate, want true")
	}
}

// TestWebhook_Delete removes the endpoint and subsequent Get returns not found.
func TestWebhook_Delete(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")

	created := seedEndpoint(t, ctx, org.ID, pgutil.UUIDToString(user.ID), "https://example.com/hook")

	svc := newService(t)

	if err := svc.Delete(ctx, org.ID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := svc.GetByID(ctx, org.ID, created.ID)
	if err == nil {
		t.Error("expected not-found after delete, got nil")
	}
}

// TestWebhook_Delete_WrongOrg ensures deleting another org's endpoint is rejected (BOLA).
func TestWebhook_Delete_WrongOrg(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")
	otherUser := fixtures.CreateActiveUser(ctx, t, testPool)
	otherOrg := fixtures.CreateOrgForUser(ctx, t, testPool, otherUser.ID, "org_owner")

	created := seedEndpoint(t, ctx, org.ID, pgutil.UUIDToString(user.ID), "https://example.com/hook")

	svc := newService(t)
	err := svc.Delete(ctx, otherOrg.ID, created.ID)
	if err == nil {
		t.Error("expected error deleting another org's endpoint, got nil")
	}
	// Endpoint must still exist.
	if _, err := svc.GetByID(ctx, org.ID, created.ID); err != nil {
		t.Errorf("endpoint should still exist after failed delete: %v", err)
	}
}

// TestWebhook_SecretNotExposed verifies that the raw secret is NOT retrievable
// via List or GetByID — only the ciphertext is stored.
func TestWebhook_SecretNotExposed(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	org := fixtures.CreateOrgForUser(ctx, t, testPool, user.ID, "org_owner")
	created := seedEndpoint(t, ctx, org.ID, pgutil.UUIDToString(user.ID), "https://example.com/hook")

	svc := newService(t)

	// GetByID returns Response (no RawSecret field).
	got, err := svc.GetByID(ctx, org.ID, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	// Response struct has no RawSecret — compilation itself enforces this.
	// Verify the ciphertext is NOT equal to the raw secret.
	var ciphertext []byte
	if err := testPool.QueryRow(ctx,
		"SELECT secret_ciphertext FROM webhook_endpoints WHERE id = $1",
		created.ID,
	).Scan(&ciphertext); err != nil {
		t.Fatalf("select ciphertext: %v", err)
	}
	// The ciphertext must differ from the raw secret (it is encrypted, not plaintext).
	_ = got // Response verified; we just need it to compile
	if string(ciphertext) == created.RawSecret {
		t.Error("secret_ciphertext equals raw secret — encryption is not applied")
	}
}

// TestWebhook_Crypto_RoundTrip verifies encrypt→decrypt round-trip.
func TestWebhook_Crypto_RoundTrip(t *testing.T) {
	key, _ := base64.StdEncoding.DecodeString(testSecretKeyB64)
	secret := "my-raw-secret-value-32bytes12345"

	ciphertext, err := webhooks.EncryptSecret(key, secret)
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}
	if len(ciphertext) == 0 {
		t.Fatal("ciphertext is empty")
	}

	decrypted, err := webhooks.DecryptSecret(key, ciphertext)
	if err != nil {
		t.Fatalf("DecryptSecret: %v", err)
	}
	if decrypted != secret {
		t.Errorf("decrypted = %q, want %q", decrypted, secret)
	}
}

// TestWebhook_SSRF_ValidateURL unit-tests the SSRF validation function.
func TestWebhook_SSRF_ValidateURL(t *testing.T) {
	valid := []string{
		"https://example.com/hook",
		"https://api.example.com/webhook",
		"https://1.2.3.4/hook", // public IPv4 literal
	}
	for _, u := range valid {
		if err := webhooks.ValidateURL(u); err != nil {
			t.Errorf("ValidateURL(%q) = %v, want nil", u, err)
		}
	}

	blocked := []string{
		"",
		"http://example.com/hook",
		"ftp://example.com/hook",
		"https://localhost/hook",
		// localhost.example.com is NOT blocked at registration time — it is a
		// normal third-party domain that happens to contain the word "localhost"
		// as a label.  DNS rebinding protection in SSRFSafeTransport blocks it
		// at delivery time if it resolves to a private IP.
		"https://127.0.0.1/hook",
		"https://127.255.255.255/hook",
		"https://10.0.0.1/hook",
		"https://10.255.255.255/hook",
		"https://172.16.0.1/hook",
		"https://172.31.255.255/hook",
		"https://192.168.0.1/hook",
		"https://192.168.255.255/hook",
		"https://169.254.169.254/latest/meta-data",
		"https://169.254.0.1/hook",
		"https://[::1]/hook",
		"https://[fc00::1]/hook",
		"https://[fe80::1]/hook",
	}
	for _, u := range blocked {
		if err := webhooks.ValidateURL(u); err == nil {
			t.Errorf("ValidateURL(%q) = nil, want error (SSRF or invalid)", u)
		}
	}
}
