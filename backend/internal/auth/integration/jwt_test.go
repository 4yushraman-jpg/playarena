package auth_integration_test

import (
	"context"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestJWT_WrongSigningKey verifies that a JWT signed with the wrong HMAC secret
// is rejected by RequireAuth before reaching any handler.
//
// Regression gate: if signature verification is bypassed (e.g. the keyFunc
// always returns nil error), the wrong-key token carries a real user ID. The
// Me handler finds the user in the DB and returns 200. assertStatus(401) then
// fails, catching the regression. Without a real user the 401 could come from
// user-not-found rather than signature failure — the baseline prevents that.
func TestJWT_WrongSigningKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Baseline: valid token for this user is accepted (200).
	// Proves the same user ID would produce 200 if security were bypassed.
	validAccess, _ := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	if apiMe(t, ts, validAccess).Email != user.Email {
		t.Fatal("baseline: /me rejected valid access token — pre-condition failed")
	}

	// Same structure, same claims, same algorithm — wrong signing secret.
	wrongKey := makeWrongKeyToken(t, uuidString(user.ID), orgID, "org_owner", user.Email)
	resp := ts.get(t, "/api/v1/auth/me", bearerHeader(wrongKey))
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
	// "authorization required" is the RequireAuth middleware's rejection body.
	// If signature verification is bypassed, the handler runs and returns
	// "unauthorized" (ErrInvalidToken from writeAuthError) — a different body.
	assertErrorBody(t, resp, "authorization required")
}

// TestJWT_EmptyEmailClaim verifies that a JWT with email = "" is rejected by
// ParseToken's explicit empty-claim check (tokens.go line 104-106) before any
// handler logic runs.
//
// Regression gate: if the claims.Email == "" check is removed from ParseToken,
// the token passes the middleware. The Me handler finds the user by user_id and
// returns 200. The test asserts 401, catching the regression.
func TestJWT_EmptyEmailClaim(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Baseline: valid token for this user is accepted (200).
	// Proves the same user ID would produce 200 if the email check were bypassed.
	validAccess, _ := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	if apiMe(t, ts, validAccess).Email != user.Email {
		t.Fatal("baseline: /me rejected valid access token — pre-condition failed")
	}

	// Correctly signed with testJWTSecret, correct issuer, valid expiry,
	// valid user_id — the only deviation is email = "".
	emptyEmail := makeEmptyEmailToken(t, uuidString(user.ID), orgID, "org_owner", testJWTSecret)
	resp := ts.get(t, "/api/v1/auth/me", bearerHeader(emptyEmail))
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
	assertErrorBody(t, resp, "authorization required")
}

// TestJWT_EmptyUserIDClaim verifies that a JWT with user_id = "" is rejected
// by ParseToken's explicit empty-claim check (tokens.go line 101-103) before
// any handler logic runs.
//
// Regression gate: if the claims.UserID == "" check is removed from ParseToken,
// the token passes the middleware. The Me handler then calls s.Me which tries
// uid.Scan("") — this fails, returning ErrInvalidToken via writeAuthError as
// "unauthorized". The test asserts "authorization required" (the middleware body),
// so a regression produces the wrong body and the assertion fails.
func TestJWT_EmptyUserIDClaim(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Baseline: valid token for this user is accepted (200).
	validAccess, _ := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	if apiMe(t, ts, validAccess).Email != user.Email {
		t.Fatal("baseline: /me rejected valid access token — pre-condition failed")
	}

	// Correctly signed with testJWTSecret, correct issuer, valid expiry —
	// the only deviation is user_id = "".
	emptyUID := makeEmptyUserIDToken(t, orgID, "org_owner", user.Email, testJWTSecret)
	resp := ts.get(t, "/api/v1/auth/me", bearerHeader(emptyUID))
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
	assertErrorBody(t, resp, "authorization required")
}
