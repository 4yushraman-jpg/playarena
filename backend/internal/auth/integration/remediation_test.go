package auth_integration_test

// remediation_test.go — integration tests added during the Phase 15A
// remediation pass. Each test targets a specific finding from the adversarial
// review and is named after its finding ID.

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

// ---- P1-3: nil-sender service bypass ----------------------------------------

// TestResendVerification_NilSender_ServiceStillCalled verifies that the
// ResendVerification service is called and creates a token even when the email
// sender is not configured. Before the P1-3 fix the entire service call was
// gated on h.emailSender != nil, so no token was ever written to the DB when
// the sender was absent.
func TestResendVerification_NilSender_ServiceStillCalled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ts := buildServerWithNilSender(t, testPool)

	user, _ := fixtures.CreatePendingUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	// Count verification tokens before the resend call.
	var countBefore int
	if err := testPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM email_verification_tokens WHERE user_id = $1 AND used_at IS NULL",
		user.ID,
	).Scan(&countBefore); err != nil {
		t.Fatalf("count tokens before: %v", err)
	}

	resp := ts.post(t, "/api/v1/auth/resend-verification", map[string]string{"email": user.Email})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	// Give the handler time to complete (it's synchronous for nil sender, but
	// the service call may still be in-flight briefly).
	time.Sleep(50 * time.Millisecond)

	var countAfter int
	if err := testPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM email_verification_tokens WHERE user_id = $1 AND used_at IS NULL",
		user.ID,
	).Scan(&countAfter); err != nil {
		t.Fatalf("count tokens after: %v", err)
	}

	if countAfter <= countBefore {
		t.Errorf("P1-3 regression: service not called — token count did not increase (before=%d after=%d)", countBefore, countAfter)
	}
}

// ---- P0-1: domain write body size protection --------------------------------

// TestBodySizeLimit_Auth_Regression is a regression guard ensuring the existing
// auth route body-size limit still fires after the middleware refactor.
func TestBodySizeLimit_Auth_Regression(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	const limit = 64 * 1024
	bigPassword := strings.Repeat("a", limit+1)
	body, _ := json.Marshal(map[string]any{
		"email":     "bodysize-regression@example.com",
		"password":  bigPassword,
		"username":  "bsregressionuser",
		"full_name": "Body Size Regression",
	})

	req, _ := http.NewRequest(http.MethodPost, ts.url+"/api/v1/auth/register", bytes.NewReader(body))
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

// ---- P2-1: Retry-After header on 429 ----------------------------------------

// TestRateLimit_RetryAfterHeader verifies that rate-limited responses include
// a Retry-After header so clients can back off correctly.
func TestRateLimit_RetryAfterHeader(t *testing.T) {
	t.Parallel()
	ts := buildRateLimitedTestServer(t, testPool, 1, 1) // 1 RPS, burst 1

	// First request consumes the burst token.
	resp1 := ts.post(t, "/api/v1/auth/login", map[string]string{
		"email": "retry-after@example.com", "password": "Password1!",
	})
	resp1.Body.Close()

	// Second request should be rate-limited.
	resp2 := ts.post(t, "/api/v1/auth/login", map[string]string{
		"email": "retry-after@example.com", "password": "Password1!",
	})
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp2.StatusCode)
	}

	retryAfter := resp2.Header.Get("Retry-After")
	if retryAfter == "" {
		t.Error("P2-1 regression: Retry-After header missing from 429 response")
	}
}

// ---- P1-4: goroutine drain --------------------------------------------------

// TestHandler_DrainEmail_CompletesWithinTimeout verifies that DrainEmail
// returns without error when no email goroutines are in-flight — the base case
// for the graceful-shutdown drain path.
func TestHandler_DrainEmail_CompletesWithinTimeout(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ts.handler.DrainEmail(ctx); err != nil {
		t.Errorf("DrainEmail with no in-flight goroutines returned error: %v", err)
	}
}

// TestRegister_DrainEmailAfterDelivery verifies that after a registration
// email is delivered, DrainEmail completes promptly — the goroutine is done
// before the drain deadline.
func TestRegister_DrainEmailAfterDelivery(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)
	ts.mailer.Reset()

	emailAddr, username, fullName := uniqueUser(t)
	reg := apiRegister(t, ts, emailAddr, "Password1!", username, fullName)

	userID := lookupUserID(context.Background(), t, reg.Email)
	t.Cleanup(func() { fixtures.CleanupUser(context.Background(), t, testPool, userID) })

	// Wait for async email delivery.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && ts.mailer.Count() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if ts.mailer.Count() == 0 {
		t.Fatal("verification email not delivered within 2s")
	}

	// After delivery the goroutine must be done; DrainEmail should return immediately.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := ts.handler.DrainEmail(ctx); err != nil {
		t.Errorf("DrainEmail after delivery: %v", err)
	}
}
