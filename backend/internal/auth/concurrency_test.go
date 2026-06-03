package auth_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// barrier returns a channel that, when closed, simultaneously unblocks all
// goroutines waiting on it. Using close(ch) rather than a WaitGroup ensures
// all goroutines contend at the same instant — no time.Sleep required.
func barrier() chan struct{} {
	return make(chan struct{})
}

// TestConcurrentRefreshSameToken starts N goroutines that all try to rotate
// the same refresh token at once. Exactly one should succeed; the rest must
// return ErrInvalidToken (Case 2 — token already rotated, no session wipe).
func TestConcurrentRefreshSameToken(t *testing.T) {
	ctx := context.Background()
	const workers = 8

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	rawOld, _ := fixtures.CreateRefreshToken(ctx, t, testPool, user.ID)
	oldHash := fixtures.HashToken(rawOld)

	repo := auth.NewRepository(db.New(testPool), testPool)

	start := barrier()
	errs := make([]error, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			newParams := db.CreateRefreshTokenParams{
				UserID:    user.ID,
				TokenHash: fixtures.HashToken(rawOld + "_rotated_by_worker_" + string(rune('0'+i))),
				ExpiresAt: auth.GetRefreshTokenExpiryTime(),
			}
			_, errs[i] = repo.RotateRefreshToken(ctx, oldHash, newParams)
		}()
	}

	close(start)
	wg.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else if !errors.Is(err, auth.ErrInvalidToken) {
			t.Errorf("unexpected error from concurrent rotation: %v", err)
		}
	}
	if successes != 1 {
		t.Errorf("expected exactly 1 successful rotation, got %d", successes)
	}
}

// TestReplayRotatedToken verifies Case 2 of the replay state machine: after a
// token is rotated, re-presenting the old token returns ErrInvalidToken
// without wiping all sessions (the legitimate holder of the successor is still
// active).
func TestReplayRotatedToken(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	rawOld, _ := fixtures.CreateRefreshToken(ctx, t, testPool, user.ID)
	oldHash := fixtures.HashToken(rawOld)

	repo := auth.NewRepository(db.New(testPool), testPool)

	// First rotation — should succeed.
	_, err := repo.RotateRefreshToken(ctx, oldHash, db.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: fixtures.HashToken("successor-1"),
		ExpiresAt: auth.GetRefreshTokenExpiryTime(),
	})
	if err != nil {
		t.Fatalf("initial rotation: %v", err)
	}

	// Replay the already-rotated token — must be Case 2 (ErrInvalidToken, no wipe).
	_, err = repo.RotateRefreshToken(ctx, oldHash, db.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: fixtures.HashToken("attacker-replay"),
		ExpiresAt: auth.GetRefreshTokenExpiryTime(),
	})
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("replay of rotated token: expected ErrInvalidToken, got %v", err)
	}
}

// TestReplayRevokedToken verifies Case 3 of the replay state machine: after a
// token is explicitly revoked (logout), re-presenting it returns ErrTokenReuse
// and wipes all active sessions for the user.
func TestReplayRevokedToken(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	// Create two tokens so we can verify that the second one is also wiped.
	rawRevoked, _ := fixtures.CreateRefreshToken(ctx, t, testPool, user.ID)
	rawSibling, _ := fixtures.CreateRefreshToken(ctx, t, testPool, user.ID)
	revokedHash := fixtures.HashToken(rawRevoked)
	siblingHash := fixtures.HashToken(rawSibling)

	repo := auth.NewRepository(db.New(testPool), testPool)

	// Logout the first token — explicit revocation, successor_id stays NULL.
	if err := repo.LogoutTransaction(ctx, revokedHash); err != nil {
		t.Fatalf("logout: %v", err)
	}

	// Replay the revoked token — must be Case 3 (ErrTokenReuse) and wipe all sessions.
	_, err := repo.RotateRefreshToken(ctx, revokedHash, db.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: fixtures.HashToken("attacker-replay-after-logout"),
		ExpiresAt: auth.GetRefreshTokenExpiryTime(),
	})
	if !errors.Is(err, auth.ErrTokenReuse) {
		t.Errorf("replay of revoked token: expected ErrTokenReuse, got %v", err)
	}

	// The sibling token must now be revoked (session wipe).
	queries := db.New(testPool)
	sibling, err := queries.GetRefreshTokenByHash(ctx, siblingHash)
	if err != nil {
		t.Fatalf("get sibling token: %v", err)
	}
	if !sibling.RevokedAt.Valid {
		t.Error("sibling token should have been revoked by session wipe")
	}
}

// TestConcurrentLogoutAndRefresh fires a logout and a rotation simultaneously
// against the same token. Both operations hold a FOR UPDATE row lock; only one
// can proceed. Whichever wins, the outcome must be consistent:
//   - If logout wins: rotation returns ErrTokenReuse (Case 3).
//   - If rotation wins: logout sees revoked_at already set and returns nil (idempotent).
//
// Neither path should panic or leave the token in an inconsistent state.
func TestConcurrentLogoutAndRefresh(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	rawToken, _ := fixtures.CreateRefreshToken(ctx, t, testPool, user.ID)
	tokenHash := fixtures.HashToken(rawToken)

	repo := auth.NewRepository(db.New(testPool), testPool)

	start := barrier()
	var logoutErr, rotateErr error
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		logoutErr = repo.LogoutTransaction(ctx, tokenHash)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, rotateErr = repo.RotateRefreshToken(ctx, tokenHash, db.CreateRefreshTokenParams{
			UserID:    user.ID,
			TokenHash: fixtures.HashToken("concurrent-new-token"),
			ExpiresAt: auth.GetRefreshTokenExpiryTime(),
		})
	}()

	close(start)
	wg.Wait()

	// Logout must have succeeded (it's idempotent even if the rotation won first).
	if logoutErr != nil {
		t.Errorf("logout error: %v", logoutErr)
	}

	// Rotation must have either succeeded or returned ErrTokenReuse (if logout won).
	// Any other error indicates an unexpected failure.
	if rotateErr != nil && !errors.Is(rotateErr, auth.ErrTokenReuse) {
		t.Errorf("rotation error: expected nil or ErrTokenReuse, got %v", rotateErr)
	}

	// The original token must be revoked regardless of which path won.
	queries := db.New(testPool)
	tok, err := queries.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("get token after concurrent ops: %v", err)
	}
	if !tok.RevokedAt.Valid {
		t.Error("token must be revoked after logout+rotation race")
	}
}

// TestConcurrentResetSameToken runs N goroutines all trying to consume the same
// password reset token. Exactly one must succeed; the rest must return
// ErrResetTokenUsed. This verifies the FOR UPDATE lock in ResetPasswordTransaction.
func TestConcurrentResetSameToken(t *testing.T) {
	ctx := context.Background()
	const workers = 6

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	rawToken := fixtures.CreatePasswordResetToken(ctx, t, testPool, user.ID)
	tokenHash := fixtures.HashToken(rawToken)

	repo := auth.NewRepository(db.New(testPool), testPool)

	// Pre-compute a new password hash once (bcrypt is slow at cost 12).
	newHash, err := auth.HashPassword("NewPassword1!")
	if err != nil {
		t.Fatalf("hash new password: %v", err)
	}

	start := barrier()
	errs := make([]error, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = repo.ResetPasswordTransaction(ctx, tokenHash, newHash)
		}()
	}

	close(start)
	wg.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else if !errors.Is(err, auth.ErrResetTokenUsed) {
			t.Errorf("unexpected error from concurrent reset: %v", err)
		}
	}
	if successes != 1 {
		t.Errorf("expected exactly 1 successful reset, got %d", successes)
	}
}

// TestConcurrentResetDifferentTokens is the deadlock regression test.
// Before Fix #1, two concurrent reset attempts each presenting a different
// token for the same user would deadlock: T1 held token A waiting for token B
// (to update siblings), T2 held token B waiting for token A — a cycle.
//
// With Fix #1 (LockUserPasswordResetTokens ORDER BY id FOR UPDATE), both
// transactions lock the same set in the same ascending order. One blocks on
// the first lock instead of creating a cycle. After the winner commits (marking
// all tokens used), the loser finds no unlocked tokens and returns ErrResetTokenUsed.
func TestConcurrentResetDifferentTokens(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	raw1 := fixtures.CreatePasswordResetToken(ctx, t, testPool, user.ID)
	raw2 := fixtures.CreatePasswordResetToken(ctx, t, testPool, user.ID)

	repo := auth.NewRepository(db.New(testPool), testPool)

	newHash, err := auth.HashPassword("NewPassword1!")
	if err != nil {
		t.Fatalf("hash new password: %v", err)
	}

	start := barrier()
	results := make([]error, 2)
	var wg sync.WaitGroup

	for i, raw := range []string{raw1, raw2} {
		i, raw := i, raw
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = repo.ResetPasswordTransaction(ctx, fixtures.HashToken(raw), newHash)
		}()
	}

	close(start)
	wg.Wait()

	// Verify: no deadlock occurred (both goroutines completed), exactly one succeeded.
	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Errorf("expected exactly 1 successful reset, got %d (results: %v, %v)",
			successes, results[0], results[1])
	}

	// The failing goroutine must have received ErrResetTokenUsed, not a raw
	// PostgreSQL deadlock error. A deadlock error here would indicate Fix #1 regressed.
	for _, err := range results {
		if err != nil && !errors.Is(err, auth.ErrResetTokenUsed) {
			t.Errorf("failed goroutine: expected ErrResetTokenUsed (not a deadlock), got: %v", err)
		}
	}
}

// TestConcurrentResetExpiredAndValidToken fires two goroutines simultaneously:
// one holding an expired token for user U, one holding a valid token for the
// same user. The valid token must always succeed; the expired token must always
// fail with ErrResetTokenExpired or ErrResetTokenUsed depending on ordering:
//
//   - If the expired goroutine locks first: it returns ErrResetTokenExpired
//     (rollback), then the valid goroutine proceeds and succeeds.
//   - If the valid goroutine locks first: it marks all tokens used, then the
//     expired goroutine finds its token gone and returns ErrResetTokenUsed.
//
// In both cases the net outcome is: exactly one success, no panics, no data
// corruption.
func TestConcurrentResetExpiredAndValidToken(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	rawExpired := fixtures.CreateExpiredPasswordResetToken(ctx, t, testPool, user.ID)
	rawValid := fixtures.CreatePasswordResetToken(ctx, t, testPool, user.ID)

	repo := auth.NewRepository(db.New(testPool), testPool)

	newHash, err := auth.HashPassword("NewPassword1!")
	if err != nil {
		t.Fatalf("hash new password: %v", err)
	}

	start := barrier()
	results := make([]error, 2) // [0] = expired goroutine, [1] = valid goroutine
	var wg sync.WaitGroup

	for i, raw := range []string{rawExpired, rawValid} {
		i, raw := i, raw
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = repo.ResetPasswordTransaction(ctx, fixtures.HashToken(raw), newHash)
		}()
	}

	close(start)
	wg.Wait()

	// The valid token goroutine must succeed.
	if results[1] != nil {
		t.Errorf("valid token goroutine: expected success, got %v", results[1])
	}

	// The expired token goroutine must fail.
	if results[0] == nil {
		t.Error("expired token goroutine: should not succeed")
	}

	// The failure must be ErrResetTokenExpired or ErrResetTokenUsed — never a
	// raw DB error, deadlock, or unexpected sentinel.
	if results[0] != nil &&
		!errors.Is(results[0], auth.ErrResetTokenExpired) &&
		!errors.Is(results[0], auth.ErrResetTokenUsed) {
		t.Errorf("expired token goroutine: expected ErrResetTokenExpired or ErrResetTokenUsed, got %v",
			results[0])
	}
}
