package auth_integration_test

import (
	"context"
	"sync"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestVerifyEmail_Success verifies that a valid, unused, unexpired verification
// token returns 200 and activates the account.
func TestVerifyEmail_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user, rawToken := fixtures.CreatePendingUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	resp := ts.get(t, "/api/v1/auth/verify-email?token="+rawToken, nil)
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var m messageResp
	decodeBody(t, resp, &m)
	if m.Message != "email verified successfully" {
		t.Errorf("verify-email: message got %q, want %q", m.Message, "email verified successfully")
	}
}

// TestVerifyEmail_EnablesLogin verifies that after email verification the user
// can successfully log in (was previously blocked by pending_verification status).
func TestVerifyEmail_EnablesLogin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user, rawToken := fixtures.CreatePendingUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Before verification: login must be rejected.
	preResp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    user.Email,
		"password": fixtures.KnownPasswordRaw,
	})
	defer preResp.Body.Close()
	assertStatus(t, preResp, 403)

	// Verify email.
	apiVerifyEmail(t, ts, rawToken)

	// After verification: login succeeds.
	_, refreshToken := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	if refreshToken == "" {
		t.Fatal("login after verification: empty refresh token")
	}
}

// TestVerifyEmail_InvalidToken verifies that a garbage token returns 400 with
// the correct error message.
func TestVerifyEmail_InvalidToken(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.get(t, "/api/v1/auth/verify-email?token=this-is-garbage", nil)
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertErrorBody(t, resp, "invalid verification token")
}

// TestVerifyEmail_ExpiredToken verifies that an already-expired token returns
// 400 with the expiry-specific error message.
func TestVerifyEmail_ExpiredToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	rawExpired := fixtures.CreateExpiredEmailVerificationToken(ctx, t, testPool, user.ID)

	resp := ts.get(t, "/api/v1/auth/verify-email?token="+rawExpired, nil)
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertErrorBody(t, resp, "verification token has expired")
}

// TestVerifyEmail_UsedToken verifies that presenting the same valid token a
// second time returns 400 "verification token has already been used".
func TestVerifyEmail_UsedToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user, rawToken := fixtures.CreatePendingUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	// First use succeeds.
	apiVerifyEmail(t, ts, rawToken)

	// Second use must be rejected.
	resp := ts.get(t, "/api/v1/auth/verify-email?token="+rawToken, nil)
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertErrorBody(t, resp, "verification token has already been used")
}

// TestVerifyEmail_MissingTokenParameter verifies that calling the verify-email
// endpoint without any ?token= query parameter returns 400 with the
// "token query parameter is required" error message.
//
// This exercises the handler's explicit empty-token guard, which is distinct
// from the ErrVerificationTokenInvalid path triggered by a non-empty but
// unrecognised token value (TestVerifyEmail_InvalidToken).
func TestVerifyEmail_MissingTokenParameter(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.get(t, "/api/v1/auth/verify-email", nil)
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertErrorBody(t, resp, "token query parameter is required")
}

// TestConcurrentEmailVerification_HTTP is the HTTP-level TOCTOU regression
// test. N goroutines simultaneously present the same verification token.
// Exactly one must receive 200; the rest must receive 400 "already been used".
//
// VerifyEmailTransaction holds a FOR UPDATE row lock so concurrent attempts
// serialise at the DB level. If the TOCTOU fix regressed, multiple goroutines
// could both observe used_at IS NULL and both succeed.
func TestConcurrentEmailVerification_HTTP(t *testing.T) {
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	const workers = 6

	user, rawToken := fixtures.CreatePendingUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	start := make(chan struct{})
	statuses := make([]int, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := doGet(ts, "/api/v1/auth/verify-email?token="+rawToken, nil)
			if err == nil {
				statuses[i] = resp.StatusCode
				resp.Body.Close()
			} else {
				statuses[i] = -1
			}
		}()
	}

	close(start)
	wg.Wait()

	successes, failures, unexpected := 0, 0, 0
	for _, code := range statuses {
		switch code {
		case 200:
			successes++
		case 400:
			failures++
		default:
			unexpected++
		}
	}

	if successes != 1 {
		t.Errorf("concurrent verify: expected exactly 1 success, got %d", successes)
	}
	if failures != workers-1 {
		t.Errorf("concurrent verify: expected %d failures (400), got %d", workers-1, failures)
	}
	if unexpected != 0 {
		t.Errorf("concurrent verify: %d unexpected status codes (not 200 or 400)", unexpected)
	}
}
