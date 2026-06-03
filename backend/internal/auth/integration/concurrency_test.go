package auth_integration_test

import (
	"context"
	"sync"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// All concurrency tests use the close(chan struct{}) barrier pattern to
// simultaneously unblock goroutines. No time.Sleep. No timing assumptions.
//
// Goroutines do not call t.Fatal/t.Errorf — they collect status codes in
// pre-allocated slices and assertions happen after wg.Wait() in the
// test goroutine.

// TestConcurrentRefresh_HTTP fires 8 goroutines that all attempt to rotate the
// same refresh token simultaneously via the HTTP endpoint. Exactly one must
// succeed (200); the rest must receive 401 (Case 2 — rotated token, no wipe).
func TestConcurrentRefresh_HTTP(t *testing.T) {
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	const workers = 8

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	_, sharedRefresh := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)

	start := make(chan struct{})
	statuses := make([]int, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := doPost(ts, "/api/v1/auth/refresh", map[string]string{
				"refresh_token": sharedRefresh,
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

	successes, invalid, unexpected := 0, 0, 0
	for _, code := range statuses {
		switch code {
		case 200:
			successes++
		case 401:
			invalid++
		default:
			unexpected++
		}
	}

	if successes != 1 {
		t.Errorf("concurrent refresh: expected exactly 1 success (200), got %d", successes)
	}
	if invalid != workers-1 {
		t.Errorf("concurrent refresh: expected %d 401 responses, got %d", workers-1, invalid)
	}
	if unexpected != 0 {
		t.Errorf("concurrent refresh: %d unexpected status codes", unexpected)
	}
}

// TestConcurrentLogoutAndRefresh_HTTP fires a logout and a rotation
// simultaneously against the same token. Both operations serialize at the
// FOR UPDATE row lock; only one can proceed. In either outcome, the original
// token must be revoked and no 5xx error must occur.
func TestConcurrentLogoutAndRefresh_HTTP(t *testing.T) {
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	_, sharedToken := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)

	start := make(chan struct{})
	logoutStatus, refreshStatus := 0, 0
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		resp, err := doPost(ts, "/api/v1/auth/logout", map[string]string{
			"refresh_token": sharedToken,
		})
		if err == nil {
			logoutStatus = resp.StatusCode
			resp.Body.Close()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		resp, err := doPost(ts, "/api/v1/auth/refresh", map[string]string{
			"refresh_token": sharedToken,
		})
		if err == nil {
			refreshStatus = resp.StatusCode
			resp.Body.Close()
		}
	}()

	close(start)
	wg.Wait()

	// Logout must always succeed (it is idempotent).
	if logoutStatus != 200 {
		t.Errorf("concurrent logout+refresh: logout got %d, want 200", logoutStatus)
	}

	// Refresh is either 200 (rotation won) or 401 (logout won → Case 3 wipe).
	if refreshStatus != 200 && refreshStatus != 401 {
		t.Errorf("concurrent logout+refresh: refresh got %d, want 200 or 401", refreshStatus)
	}

	// The original token must be unusable regardless of which path won.
	verifyResp, err := doPost(ts, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": sharedToken,
	})
	if err != nil {
		t.Fatalf("post-race verify: %v", err)
	}
	verifyResp.Body.Close()
	if verifyResp.StatusCode != 401 {
		t.Errorf("post-race: original token should be revoked (401), got %d",
			verifyResp.StatusCode)
	}
}

// TestConcurrentPasswordReset_DifferentTokens_HTTP verifies that two goroutines
// each presenting a different reset token for the same user do not deadlock.
// Exactly one must succeed; the other receives 400 (not 500).
//
// This is the HTTP-level complement to TestConcurrentResetDifferentTokens in
// the repository concurrency suite. A 500 response here would indicate that the
// PostgreSQL deadlock error is escaping through the service layer.
func TestConcurrentPasswordReset_DifferentTokens_HTTP(t *testing.T) {
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	raw1 := fixtures.CreatePasswordResetToken(ctx, t, testPool, user.ID)
	raw2 := fixtures.CreatePasswordResetToken(ctx, t, testPool, user.ID)

	start := make(chan struct{})
	statuses := make([]int, 2)
	var wg sync.WaitGroup

	for i, raw := range []string{raw1, raw2} {
		i, raw := i, raw
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := doPost(ts, "/api/v1/auth/reset-password", map[string]string{
				"token":    raw,
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
		t.Errorf("concurrent reset (diff tokens): expected exactly 1 success, got %d", successes)
	}
	if failures != 1 {
		t.Errorf("concurrent reset (diff tokens): expected exactly 1 failure (400), got %d", failures)
	}
	if unexpected != 0 {
		// 500 here = deadlock leaked to HTTP. Fix #1 (LockUserPasswordResetTokens
		// ORDER BY id) must have regressed.
		t.Errorf("concurrent reset (diff tokens): unexpected status codes: %v", statuses)
	}
}
