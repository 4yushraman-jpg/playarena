package auth_test

import (
	"context"
	"testing"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestRevokeAndLinkSuccessorInvariant verifies the structural invariant that
// underpins the replay detection state machine (Auth Security Hotfix v2):
//
//   - After a successful token rotation the OLD token must have both
//     revoked_at IS NOT NULL and successor_id == the new token's ID.
//   - The NEW token must have revoked_at IS NULL and successor_id IS NULL
//     (it is the active token).
//
// If RevokeAndLinkSuccessor ever failed to set successor_id, a legitimately
// rotated token would be mis-classified as Case 3 (explicitly revoked) and
// trigger a full session wipe on the next legitimate refresh attempt.
func TestRevokeAndLinkSuccessorInvariant(t *testing.T) {
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	// Create the token that will be rotated.
	rawOld, _ := fixtures.CreateRefreshToken(ctx, t, testPool, user.ID)
	oldHash := fixtures.HashToken(rawOld)

	// Prepare the params for the replacement token.
	rawNew := "synthetic-new-token-for-invariant-test"
	newParams := db.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: fixtures.HashToken(rawNew),
		ExpiresAt: auth.GetRefreshTokenExpiryTime(),
		IpAddress: nil,
		UserAgent: nil,
	}

	repo := auth.NewRepository(db.New(testPool), testPool)

	rotated, err := repo.RotateRefreshToken(ctx, oldHash, newParams)
	if err != nil {
		t.Fatalf("RotateRefreshToken: %v", err)
	}

	// Verify the OLD token's post-rotation state.
	queries := db.New(testPool)
	oldTok, err := queries.GetRefreshTokenByHash(ctx, oldHash)
	if err != nil {
		t.Fatalf("get old token after rotation: %v", err)
	}

	// Invariant 1: old token must be revoked.
	if !oldTok.RevokedAt.Valid {
		t.Error("old token: revoked_at should be set after rotation")
	}

	// Invariant 2: old token must carry the new token's ID as successor.
	// This is the Case 2 classification signal: a concurrent re-use of the
	// old token returns ErrInvalidToken without wiping sessions, because the
	// legitimate client still holds the successor token.
	if !oldTok.SuccessorID.Valid {
		t.Fatal("old token: successor_id should be set after rotation (Case 2 classification)")
	}
	if oldTok.SuccessorID.Bytes != rotated.ID.Bytes {
		t.Errorf("old token: successor_id = %x, want %x",
			oldTok.SuccessorID.Bytes, rotated.ID.Bytes)
	}

	// Invariant 3: the new (rotated) token must be active.
	if rotated.RevokedAt.Valid {
		t.Error("new token: revoked_at should be NULL (token is still active)")
	}
	if rotated.SuccessorID.Valid {
		t.Error("new token: successor_id should be NULL (not yet rotated)")
	}
}
