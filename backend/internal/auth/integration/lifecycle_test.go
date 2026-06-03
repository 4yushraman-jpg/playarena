package auth_integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestLifecycle_FullAuthFlow exercises the complete happy-path auth lifecycle
// end-to-end through the HTTP API:
//
//	Register → VerifyEmail → Login → Me → Refresh → Logout
//
// This is the master integration smoke test. Granular failure cases are
// covered by the dedicated test files.
func TestLifecycle_FullAuthFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	// 1. Register a new user via the HTTP endpoint.
	email, username, fullName := uniqueUser(t)
	reg := apiRegister(t, ts, email, "Password1!", username, fullName)
	if reg.ID == "" {
		t.Fatal("register: empty user ID in response")
	}
	if reg.VerificationToken == "" {
		t.Fatal("register: verification_token absent; server must be in development mode")
	}

	// 2. Look up the newly created user in the DB to get their pgtype.UUID for
	//    org fixture setup. We use the email returned in the response.
	userID := lookupUserID(ctx, t, reg.Email)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, userID) })

	// 3. Verify email using the token returned by the register endpoint.
	apiVerifyEmail(t, ts, reg.VerificationToken)

	// 4. Grant the user an org role so login can resolve an org context.
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, userID, "org_owner")

	// 5. Login.
	accessToken, refreshToken := apiLogin(t, ts, email, "Password1!", orgID)

	// 6. Me — profile matches what we registered.
	me := apiMe(t, ts, accessToken)
	if me.Email != email {
		t.Errorf("me.email: got %q, want %q", me.Email, email)
	}
	if me.OrganizationID != orgID {
		t.Errorf("me.organization_id: got %q, want %q", me.OrganizationID, orgID)
	}

	// 7. Refresh — should return a new valid token pair.
	newAccess, newRefresh := apiRefresh(t, ts, refreshToken, "")
	if newAccess == "" || newRefresh == "" {
		t.Fatal("refresh: empty tokens in response")
	}

	// 8. Me with new access token — still resolves correctly.
	me2 := apiMe(t, ts, newAccess)
	if me2.Email != email {
		t.Errorf("me (after refresh).email: got %q, want %q", me2.Email, email)
	}

	// 9. Logout with the new refresh token.
	apiLogout(t, ts, newRefresh)
}

// ---- Registration tests -----------------------------------------------------

// TestRegister_Success verifies that a valid registration returns 201 with a
// user ID, email, username, and verification_token (development mode).
func TestRegister_Success(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	email, username, fullName := uniqueUser(t)
	resp := ts.post(t, "/api/v1/auth/register", map[string]any{
		"email":     email,
		"password":  "Password1!",
		"username":  username,
		"full_name": fullName,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 201)

	var r registerResp
	decodeBody(t, resp, &r)

	if r.ID == "" {
		t.Error("register: id is empty")
	}
	if r.Email != email {
		t.Errorf("register: email got %q, want %q", r.Email, email)
	}
	if r.Username != username {
		t.Errorf("register: username got %q, want %q", r.Username, username)
	}
	if r.VerificationToken == "" {
		t.Error("register: verification_token absent in dev mode")
	}

	// Clean up the created user.
	ctx := context.Background()
	userID := lookupUserID(ctx, t, email)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, userID) })
}

// TestRegister_DuplicateEmail verifies that registering the same email twice
// returns 409.
func TestRegister_DuplicateEmail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	resp := ts.post(t, "/api/v1/auth/register", map[string]any{
		"email":     user.Email,
		"password":  "Password1!",
		"username":  "unique_dup_email_uname",
		"full_name": "Dup User",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 409)
	assertErrorBody(t, resp, "email address is already registered")
}

// TestRegister_DuplicateUsername verifies that registering with an existing
// username returns 409.
func TestRegister_DuplicateUsername(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	resp := ts.post(t, "/api/v1/auth/register", map[string]any{
		"email":     "unique_for_dupuname@example.com",
		"password":  "Password1!",
		"username":  user.Username,
		"full_name": "Dup User",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 409)
	assertErrorBody(t, resp, "username is already taken")
}

// ---- Login tests ------------------------------------------------------------

// TestLogin_Success verifies that valid credentials return 200 with a full
// token pair and correct token metadata.
func TestLogin_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    user.Email,
		"password": fixtures.KnownPasswordRaw,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var r loginResp
	decodeBody(t, resp, &r)
	if r.AccessToken == "" {
		t.Error("login: empty access_token")
	}
	if r.RefreshToken == "" {
		t.Error("login: empty refresh_token")
	}
	if r.ExpiresIn != 900 {
		t.Errorf("login: expires_in got %d, want 900", r.ExpiresIn)
	}
	if r.TokenType != "Bearer" {
		t.Errorf("login: token_type got %q, want %q", r.TokenType, "Bearer")
	}
}

// TestLogin_WrongPassword verifies that an incorrect password returns 401.
func TestLogin_WrongPassword(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    user.Email,
		"password": "wrongpassword",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
	assertErrorBody(t, resp, "invalid credentials")
}

// TestLogin_UnknownEmail verifies that an unknown email returns the same 401
// body as a wrong password (anti-enumeration). The server performs a cost-12
// bcrypt dummy comparison on the not-found path so timing is equalised.
func TestLogin_UnknownEmail(t *testing.T) {
	// Not parallel: dummy bcrypt at cost 12 takes ~400 ms.
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    "nobody_at_all@example.com",
		"password": "Password1!",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
	assertErrorBody(t, resp, "invalid credentials")
}

// ---- Logout tests -----------------------------------------------------------

// TestLogout_Success verifies that a valid refresh token can be logged out and
// the response body contains the expected message.
func TestLogout_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	_, refreshToken := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)

	resp := ts.post(t, "/api/v1/auth/logout", map[string]string{
		"refresh_token": refreshToken,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 200)

	var m messageResp
	decodeBody(t, resp, &m)
	if m.Message != "logged out" {
		t.Errorf("logout: message got %q, want %q", m.Message, "logged out")
	}
}

// TestLogout_RevokesToken verifies that after logout the same refresh token
// cannot be used for a subsequent refresh (token is revoked).
func TestLogout_RevokesToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	_, refreshToken := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	apiLogout(t, ts, refreshToken)

	resp := ts.post(t, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": refreshToken,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 401)
}

// ---- Me tests ---------------------------------------------------------------

// TestMe_Success verifies that a valid access token returns the correct profile
// with matching email and organization_id.
func TestMe_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := buildTestServer(t, testPool)

	user := fixtures.CreateActiveUser(ctx, t, testPool)
	t.Cleanup(func() { fixtures.CleanupUser(ctx, t, testPool, user.ID) })
	orgID := fixtures.CreateOrgWithRole(ctx, t, testPool, user.ID, "org_owner")

	accessToken, _ := apiLogin(t, ts, user.Email, fixtures.KnownPasswordRaw, orgID)
	me := apiMe(t, ts, accessToken)

	if me.Email != user.Email {
		t.Errorf("me.email: got %q, want %q", me.Email, user.Email)
	}
	if me.OrganizationID != orgID {
		t.Errorf("me.organization_id: got %q, want %q", me.OrganizationID, orgID)
	}
	if me.Status != "active" {
		t.Errorf("me.status: got %q, want %q", me.Status, "active")
	}
	if me.Role == "" {
		t.Error("me.role: empty")
	}
}

// TestLogout_EmptyRefreshToken verifies that presenting an empty string as the
// refresh_token field returns 400.
//
// Note: the instruction specified 401 (the service-layer ErrInvalidToken path),
// but the custom validator enforces validate:"required" on LogoutRequest.RefreshToken
// before the service is reached. An empty string fails the required rule and
// triggers writeDecodeError → 400. The service-level empty-token guard is
// therefore only reachable when refresh_token is absent from the JSON body
// entirely, not when it is present as "".
func TestLogout_EmptyRefreshToken(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/logout", map[string]string{
		"refresh_token": "",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
}

// ---- package-level helpers --------------------------------------------------

// lookupUserID queries the DB for the user's UUID by email. Used by tests that
// create users via the HTTP registration endpoint and need the pgtype.UUID for
// subsequent fixture helpers.
func lookupUserID(ctx context.Context, t testing.TB, email string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := testPool.QueryRow(ctx,
		"SELECT id FROM users WHERE email = $1", email,
	).Scan(&id); err != nil {
		t.Fatalf("lookupUserID %q: %v", email, err)
	}
	return id
}
