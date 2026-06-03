package auth_integration_test

import (
	"context"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ---- RequireAuth middleware tests -------------------------------------------

// TestMe_NoAuthHeader verifies that a request to a protected endpoint without
// an Authorization header returns 401.
func TestMe_NoAuthHeader(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.get(t, "/api/v1/auth/me", nil)
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
	assertErrorBody(t, resp, "authorization required")
}

// TestMe_ExpiredToken verifies that a correctly signed but expired access token
// returns 401 (ErrExpiredToken path in RequireAuth).
//
// A real fixture user is used so that the baseline confirms a valid token from
// the same user IS accepted (200). This ensures the 401 below comes from the
// expiry gate in ValidateToken, not from user-not-found — which would produce
// 401 regardless of whether the expiry check exists.
func TestMe_ExpiredToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Baseline: a valid token for this user must be accepted.
	// If this fails, the test pre-condition is broken, not the expiry gate.
	validAccess, _ := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	if apiMe(t, ts, validAccess).Email != user.Email {
		t.Fatal("baseline: /me rejected valid access token — pre-condition failed")
	}

	expired := makeExpiredToken(t, uuidString(user.ID), orgID, "org_owner", user.Email, testJWTSecret)
	resp := ts.get(t, "/api/v1/auth/me", bearerHeader(expired))
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
	assertErrorBody(t, resp, "authorization required")
}

// TestMe_TamperedToken verifies that a token with a modified payload (invalid
// signature) is rejected with 401.
func TestMe_TamperedToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	valid, _ := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	tampered := makeTamperedToken(t, valid)

	resp := ts.get(t, "/api/v1/auth/me", bearerHeader(tampered))
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
	assertErrorBody(t, resp, "authorization required")
}

// TestMiddleware_RequireAuth_AlgorithmConfusion verifies that a token signed
// with HS512 (instead of HS256) is rejected (algorithm confusion attack).
//
// A real fixture user is used so the baseline proves a valid HS256 token IS
// accepted. This means a regression removing WithValidMethods would cause the
// HS512 token to pass JWT validation and reach the Me handler, which would
// return 200 for the real user — making assertStatus(401) fail and catching
// the regression. Without a real user the 401 would come from user-not-found.
func TestMiddleware_RequireAuth_AlgorithmConfusion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Baseline: HS256 token accepted.
	validAccess, _ := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	if apiMe(t, ts, validAccess).Email != user.Email {
		t.Fatal("baseline: /me rejected valid access token — pre-condition failed")
	}

	tok := makeAlgorithmConfusionToken(t,
		uuidString(user.ID), orgID, "org_owner", user.Email,
		testJWTSecret,
	)
	resp := ts.get(t, "/api/v1/auth/me", bearerHeader(tok))
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
	assertErrorBody(t, resp, "authorization required")
}

// TestMiddleware_RequireAuth_WrongIssuer verifies that a correctly signed HS256
// token with iss != "playarena" is rejected.
//
// Same real-user rationale as TestMiddleware_RequireAuth_AlgorithmConfusion:
// if WithIssuer("playarena") is removed from ParseToken, the wrong-issuer token
// would pass JWT validation and the Me handler would return 200 for the real
// user, making assertStatus(401) fail and catching the regression.
func TestMiddleware_RequireAuth_WrongIssuer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	// Baseline: correct issuer accepted.
	validAccess, _ := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	if apiMe(t, ts, validAccess).Email != user.Email {
		t.Fatal("baseline: /me rejected valid access token — pre-condition failed")
	}

	tok := makeWrongIssuerToken(t,
		uuidString(user.ID), orgID, "org_owner", user.Email,
		testJWTSecret,
	)
	resp := ts.get(t, "/api/v1/auth/me", bearerHeader(tok))
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
	assertErrorBody(t, resp, "authorization required")
}

// ---- RequirePermission middleware tests -------------------------------------

// TestMiddleware_RequirePermission_Granted verifies that a user whose role
// includes "role.assign" can access the admin-only endpoint (200).
// org_owner holds role.assign, so we use that role for the test user.
func TestMiddleware_RequirePermission_Granted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	accessToken, _ := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)

	resp := ts.get(t, "/api/v1/auth/admin-only", bearerHeader(accessToken))
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
}

// TestMiddleware_RequirePermission_Denied verifies that a user whose role does
// NOT include "role.assign" receives 403 on the admin-only endpoint.
// viewer role has no management permissions.
func TestMiddleware_RequirePermission_Denied(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "viewer")

	accessToken, _ := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)

	resp := ts.get(t, "/api/v1/auth/admin-only", bearerHeader(accessToken))
	defer resp.Body.Close()
	assertStatus(t, resp, 403)
	assertErrorBody(t, resp, "insufficient permissions")
}
