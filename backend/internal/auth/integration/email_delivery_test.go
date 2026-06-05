package auth_integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ---- Email delivery on register ---------------------------------------------

// TestRegister_SendsVerificationEmail asserts that a successful registration
// triggers exactly one email sent to the registered address via the
// NoOpProvider wired into the test server.
func TestRegister_SendsVerificationEmail(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)
	ts.mailer.Reset()

	emailAddr, username, fullName := uniqueUser(t)
	reg := apiRegister(t, ts, emailAddr, "Password1!", username, fullName)

	userID := lookupUserID(context.Background(), t, reg.Email)
	t.Cleanup(func() { fixtures.CleanupUser(context.Background(), t, testPool, userID) })

	// Give the async goroutine in the handler time to deliver the email.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && ts.mailer.Count() == 0 {
		time.Sleep(10 * time.Millisecond)
	}

	msgs := ts.mailer.SentTo(emailAddr)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 verification email to %q, got %d", emailAddr, len(msgs))
	}
	if msgs[0].Subject == "" {
		t.Error("verification email: subject must not be empty")
	}
}

// TestRegister_VerificationEmailContainsLink asserts that the verification
// email body contains the verify-email URL with the correct base URL.
func TestRegister_VerificationEmailContainsLink(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)
	ts.mailer.Reset()

	emailAddr, username, fullName := uniqueUser(t)
	reg := apiRegister(t, ts, emailAddr, "Password1!", username, fullName)

	userID := lookupUserID(context.Background(), t, reg.Email)
	t.Cleanup(func() { fixtures.CleanupUser(context.Background(), t, testPool, userID) })

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && ts.mailer.Count() == 0 {
		time.Sleep(10 * time.Millisecond)
	}

	msgs := ts.mailer.SentTo(emailAddr)
	if len(msgs) == 0 {
		t.Fatal("no verification email delivered")
	}
	m := msgs[0]
	const wantPrefix = "http://localhost:8080/verify-email?token="
	found := strings.Contains(m.TextBody, wantPrefix) || strings.Contains(m.HTMLBody, wantPrefix)
	if !found {
		t.Errorf("verify link not found in email body; text=%q", m.TextBody)
	}
}

// ---- Body size limit --------------------------------------------------------

// TestBodySizeLimit_Auth asserts that an oversized request body to an auth
// endpoint is rejected with 413 Request Entity Too Large before any handler
// logic runs.
//
// The limit is 64 KB. A 65 KB body must be rejected.
func TestBodySizeLimit_Auth(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	// Build a JSON body that exceeds 64 KB. We embed the excess bytes in the
	// password field (which accepts any string the validator has not yet seen).
	const limit = 64 * 1024
	bigPassword := strings.Repeat("a", limit+1)
	body, _ := json.Marshal(map[string]any{
		"email":     "bodysize@example.com",
		"password":  bigPassword,
		"username":  "bodysizeuser",
		"full_name": "Body Size Test",
	})

	req, err := http.NewRequest(http.MethodPost, ts.url+"/api/v1/auth/register", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /register: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", resp.StatusCode)
	}
}

// ---- Resend verification ----------------------------------------------------

// TestResendVerification_AlwaysReturns200 verifies that the resend-verification
// endpoint returns HTTP 200 for any email address — registered, unknown, or
// already-active. The response body must not expose whether the address exists.
func TestResendVerification_AlwaysReturns200(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	cases := []string{
		"nonexistent@example.com",
		"another-nonexistent@example.com",
	}
	for _, addr := range cases {
		resp := ts.post(t, "/api/v1/auth/resend-verification", map[string]string{"email": addr})
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)
	}
}

// TestResendVerification_ActiveAccountReturns200 verifies that calling
// resend-verification for an already-active (verified) account still returns
// 200 — enumeration resistance applies to active accounts too.
func TestResendVerification_ActiveAccountReturns200(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	resp := ts.post(t, "/api/v1/auth/resend-verification", map[string]string{"email": user.Email})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
}

// TestResendVerification_PendingUserTriggersEmail verifies that resend for a
// pending (unverified) user results in exactly one additional email being
// dispatched via the NoOpProvider.
func TestResendVerification_PendingUserTriggersEmail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)
	ts.mailer.Reset()

	user, _ := fixtures.CreatePendingUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	resp := ts.post(t, "/api/v1/auth/resend-verification", map[string]string{"email": user.Email})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	// Wait for the async goroutine to deliver the email.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && ts.mailer.Count() == 0 {
		time.Sleep(10 * time.Millisecond)
	}

	msgs := ts.mailer.SentTo(user.Email)
	if len(msgs) != 1 {
		t.Errorf("expected 1 resend email, got %d", len(msgs))
	}
}

// TestResendVerification_InvalidEmailFormat verifies that an invalid email
// format returns 400 from the validator — NOT 200. The always-200 guarantee
// applies only to service-level outcomes (unknown email, already active), not
// to validator rejections that occur before the service is called.
func TestResendVerification_InvalidEmailFormat(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/resend-verification", map[string]string{"email": "not-an-email"})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
	assertValidationError(t, resp, "email")
}

// TestResendVerification_MissingEmail verifies that omitting the email field
// returns 400 with fields.email populated (required rule).
func TestResendVerification_MissingEmail(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/resend-verification", map[string]string{})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
	assertValidationError(t, resp, "email")
}

// TestResendVerification_ResponseMessage checks the exact message body so a
// future refactor that changes the wording is caught before deploy.
func TestResendVerification_ResponseMessage(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/resend-verification", map[string]string{
		"email": "nobody@example.com",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var body messageResp
	decodeBody(t, resp, &body)
	const want = "if the email is registered and unverified, a new verification link has been sent"
	if body.Message != want {
		t.Errorf("message: got %q, want %q", body.Message, want)
	}
}
