package auth_integration_test

import (
	"context"
	"sync"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestForgotPassword_KnownEmail verifies that a registered email returns 200
// with a reset token in development mode.
func TestForgotPassword_KnownEmail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	resp := ts.post(t, "/api/v1/auth/forgot-password", map[string]string{
		"email": user.Email,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var r forgotPasswordResp
	decodeBody(t, resp, &r)
	if r.Message == "" {
		t.Error("forgot-password: empty message")
	}
	if r.ResetToken == "" {
		t.Error("forgot-password: reset_token absent in dev mode")
	}
}

// TestForgotPassword_UnknownEmail verifies that an unregistered email returns
// the same 200 response as a known email (anti-enumeration). The server
// performs timing equalization (4 DB round-trips) on the not-found path.
func TestForgotPassword_UnknownEmail(t *testing.T) {
	// Not parallel: relies on timing equalization. No parallel issue, but
	// serializing ensures the equalization goroutine's DB round-trips don't
	// compete with other tests' bcrypt operations for the connection pool.
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/forgot-password", map[string]string{
		"email": "nobody_registered@example.com",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var r forgotPasswordResp
	decodeBody(t, resp, &r)
	if r.Message == "" {
		t.Error("forgot-password (unknown): empty message")
	}
	// reset_token must be absent for unregistered emails.
	if r.ResetToken != "" {
		t.Errorf("forgot-password (unknown): reset_token must be absent, got %q", r.ResetToken)
	}
}

// TestResetPassword_Success verifies the complete reset flow: request token,
// consume it, and confirm 200 with the expected message.
func TestResetPassword_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	rawToken := apiForgotPassword(t, ts, user.Email)

	resp := ts.post(t, "/api/v1/auth/reset-password", map[string]string{
		"token":    rawToken,
		"password": "NewPassword2!",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var m messageResp
	decodeBody(t, resp, &m)
	if m.Message == "" {
		t.Error("reset-password: empty message")
	}
}

// TestResetPassword_ExpiredToken verifies that presenting an already-expired
// reset token returns 400 with the expiry error message.
func TestResetPassword_ExpiredToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	rawExpired := fixtures.CreateExpiredPasswordResetToken(ctx, t, testPool, user.ID)

	resp := ts.post(t, "/api/v1/auth/reset-password", map[string]string{
		"token":    rawExpired,
		"password": "NewPassword2!",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertErrorBody(t, resp, "password reset token has expired")
}

// TestResetPassword_UsedToken verifies that presenting a token that has already
// been consumed returns 400 "password reset token has already been used".
func TestResetPassword_UsedToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	rawToken := apiForgotPassword(t, ts, user.Email)
	apiResetPassword(t, ts, rawToken, "NewPassword2!")

	// Second use of the same token must fail.
	resp := ts.post(t, "/api/v1/auth/reset-password", map[string]string{
		"token":    rawToken,
		"password": "AnotherPassword3!",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertErrorBody(t, resp, "password reset token has already been used")
}

// TestResetPassword_InvalidToken verifies that a garbage token string returns
// 400 "invalid password reset token".
func TestResetPassword_InvalidToken(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/reset-password", map[string]string{
		"token":    "this-is-not-a-valid-reset-token",
		"password": "NewPassword2!",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertErrorBody(t, resp, "invalid password reset token")
}

// TestResetPassword_RevokesAllSessions verifies that after a successful
// password reset all previously issued refresh tokens are revoked, preventing
// concurrent sessions from surviving the reset.
func TestResetPassword_RevokesAllSessions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Open three independent sessions.
	_, session1 := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	_, session2 := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	_, session3 := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)

	rawToken := apiForgotPassword(t, ts, user.Email)
	apiResetPassword(t, ts, rawToken, "NewPassword2!")

	for _, sess := range []string{session1, session2, session3} {
		resp := ts.post(t, "/api/v1/auth/refresh", map[string]string{
			"refresh_token": sess,
		})
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Errorf("expected session revoked (401), got %d", resp.StatusCode)
		}
	}
}

// TestResetPassword_OldPasswordFails verifies that after a successful reset the
// old password no longer authenticates.
func TestResetPassword_OldPasswordFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	rawToken := apiForgotPassword(t, ts, user.Email)
	apiResetPassword(t, ts, rawToken, "NewPassword2!")

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    user.Email,
		"password": fixtures.KnownPasswordRaw,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
	assertErrorBody(t, resp, "invalid credentials")
}

// TestResetPassword_NewPasswordWorks verifies that after a successful reset the
// new password authenticates correctly.
func TestResetPassword_NewPasswordWorks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	rawToken := apiForgotPassword(t, ts, user.Email)
	apiResetPassword(t, ts, rawToken, "NewPassword2!")

	// New password must succeed.
	accessToken, _ := apiLogin(t, ts, user.Email, "NewPassword2!", orgID)
	if accessToken == "" {
		t.Fatal("login with new password: empty access token")
	}
}

// TestConcurrentPasswordReset_HTTP is the HTTP-level complement to
// TestConcurrentResetSameToken. N goroutines simultaneously present the same
// reset token. Exactly one must succeed (200); the rest must receive 400 (not
// 500 — no deadlock or internal error must surface to callers).
func TestConcurrentPasswordReset_HTTP(t *testing.T) {
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	const workers = 6

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	rawToken := fixtures.CreatePasswordResetToken(ctx, t, testPool, user.ID)

	start := make(chan struct{})
	statuses := make([]int, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := doPost(ts, "/api/v1/auth/reset-password", map[string]string{
				"token":    rawToken,
				"password": "NewPassword2!",
			})
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
		t.Errorf("concurrent reset: expected exactly 1 success, got %d", successes)
	}
	if unexpected != 0 {
		// 500 here would indicate a deadlock reaching the HTTP layer.
		t.Errorf("concurrent reset: %d responses with unexpected status codes (not 200 or 400)", unexpected)
	}
}
