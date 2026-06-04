package auth_integration_test

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/4yushraman-jpg/playarena/internal/auth"
)

// ---- API-level auth flow helpers --------------------------------------------
// These helpers drive real HTTP endpoints and use t.Fatal on unexpected failure.
// They are designed for the happy path; negative tests call ts.post/ts.get directly.

// apiRegister calls POST /api/v1/auth/register and returns the registration
// response on 201. Fails the test on any other status.
func apiRegister(t testing.TB, ts *testServer, email, password, username, fullName string) registerResp {
	t.Helper()
	resp := ts.post(t, "/api/v1/auth/register", map[string]any{
		"email":     email,
		"password":  password,
		"username":  username,
		"full_name": fullName,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 201)
	var r registerResp
	decodeBody(t, resp, &r)
	return r
}

// apiVerifyEmail calls GET /api/v1/auth/verify-email?token=<rawToken> and
// asserts 200.
func apiVerifyEmail(t testing.TB, ts *testServer, rawToken string) {
	t.Helper()
	resp := ts.get(t, "/api/v1/auth/verify-email?token="+rawToken, nil)
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
}

// apiLogin calls POST /api/v1/auth/login and returns the access and refresh
// tokens on success. orgID may be empty for single-org / platform-admin users.
func apiLogin(t testing.TB, ts *testServer, email, password, orgID string) (accessToken, refreshToken string) {
	t.Helper()
	body := map[string]any{"email": email, "password": password}
	if orgID != "" {
		body["organization_id"] = orgID
	}
	resp := ts.post(t, "/api/v1/auth/login", body)
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
	var r loginResp
	decodeBody(t, resp, &r)
	if r.AccessToken == "" {
		t.Fatal("apiLogin: empty access_token in response")
	}
	if r.RefreshToken == "" {
		t.Fatal("apiLogin: empty refresh_token in response")
	}
	return r.AccessToken, r.RefreshToken
}

// apiRefresh calls POST /api/v1/auth/refresh and returns the new token pair.
// orgID may be empty to keep the existing org context.
func apiRefresh(t testing.TB, ts *testServer, rawRefreshToken, orgID string) (accessToken, refreshToken string) {
	t.Helper()
	body := map[string]any{"refresh_token": rawRefreshToken}
	if orgID != "" {
		body["organization_id"] = orgID
	}
	resp := ts.post(t, "/api/v1/auth/refresh", body)
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
	var r loginResp
	decodeBody(t, resp, &r)
	return r.AccessToken, r.RefreshToken
}

// apiLogout calls POST /api/v1/auth/logout and asserts 200.
func apiLogout(t testing.TB, ts *testServer, rawRefreshToken string) {
	t.Helper()
	resp := ts.post(t, "/api/v1/auth/logout", map[string]string{
		"refresh_token": rawRefreshToken,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
}

// apiForgotPassword calls POST /api/v1/auth/forgot-password and returns the
// raw reset token. In development mode the token is returned in the body.
func apiForgotPassword(t testing.TB, ts *testServer, email string) string {
	t.Helper()
	resp := ts.post(t, "/api/v1/auth/forgot-password", map[string]string{"email": email})
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
	var r forgotPasswordResp
	decodeBody(t, resp, &r)
	if r.ResetToken == "" {
		t.Fatal("apiForgotPassword: empty reset_token; is server in development mode?")
	}
	return r.ResetToken
}

// apiResetPassword calls POST /api/v1/auth/reset-password and asserts 200.
func apiResetPassword(t testing.TB, ts *testServer, rawToken, newPassword string) {
	t.Helper()
	resp := ts.post(t, "/api/v1/auth/reset-password", map[string]string{
		"token":    rawToken,
		"password": newPassword,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
}

// apiMe calls GET /api/v1/auth/me with the given access token and returns the
// me response on 200.
func apiMe(t testing.TB, ts *testServer, accessToken string) meResp {
	t.Helper()
	resp := ts.get(t, "/api/v1/auth/me", bearerHeader(accessToken))
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
	var r meResp
	decodeBody(t, resp, &r)
	return r
}

// ---- Token construction helpers (for middleware tests) ----------------------

const jwtIssuer = "playarena"

// uuidString formats a pgtype.UUID as the standard hyphenated UUID string
// (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx). Used by middleware tests that must
// construct JWT tokens carrying a real user's UUID without importing pgutil.
func uuidString(u pgtype.UUID) string {
	b := u.Bytes
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}

// makeExpiredToken generates a correctly-signed HS256 token whose exp is in
// the past, triggering ErrExpiredToken validation.
func makeExpiredToken(t testing.TB, userID, orgID, role, email, secret string) string {
	t.Helper()
	now := time.Now()
	claims := auth.JWTClaims{
		UserID:         userID,
		OrganizationID: orgID,
		Role:           role,
		Email:          email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now.Add(-16 * time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-16 * time.Minute)),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("makeExpiredToken: %v", err)
	}
	return tok
}

// makeTamperedToken takes a valid token string and flips a byte in its
// payload segment so the signature no longer matches.
func makeTamperedToken(t testing.TB, validToken string) string {
	t.Helper()
	b := []byte(validToken)
	// Flip a character in the middle of the payload (second segment).
	mid := len(b) / 2
	if b[mid] == 'a' {
		b[mid] = 'b'
	} else {
		b[mid] = 'a'
	}
	return string(b)
}

// makeAlgorithmConfusionToken generates a token signed with HS512 rather than
// HS256. ValidateToken must reject it (algorithm confusion attack).
func makeAlgorithmConfusionToken(t testing.TB, userID, orgID, role, email, secret string) string {
	t.Helper()
	now := time.Now()
	claims := auth.JWTClaims{
		UserID:         userID,
		OrganizationID: orgID,
		Role:           role,
		Email:          email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("makeAlgorithmConfusionToken: %v", err)
	}
	return tok
}

// testWrongKey is a signing secret distinct from testJWTSecret. Any token
// signed with this key will fail HMAC verification against testJWTSecret.
const testWrongKey = "playarena-wrong-signing-key-for-testing!!"

// makeWrongKeyToken generates a fully valid HS256 JWT — correct structure,
// correct claims, correct algorithm, correct issuer — but signed with
// testWrongKey instead of testJWTSecret. The server rejects it at the HMAC
// verification step inside ParseToken.
func makeWrongKeyToken(t testing.TB, userID, orgID, role, email string) string {
	t.Helper()
	now := time.Now()
	claims := auth.JWTClaims{
		UserID:         userID,
		OrganizationID: orgID,
		Role:           role,
		Email:          email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testWrongKey))
	if err != nil {
		t.Fatalf("makeWrongKeyToken: %v", err)
	}
	return tok
}

// makeEmptyUserIDToken generates a correctly-signed HS256 JWT with user_id set
// to the empty string. ParseToken's explicit claims check
// (`claims.UserID == "" → ErrInvalidToken`) must reject it. If that check is
// removed, the token passes to the Me handler, which fails at uid.Scan("") and
// returns "unauthorized" instead of "authorization required" — making tests
// that assert the latter body catch the regression.
func makeEmptyUserIDToken(t testing.TB, orgID, role, email, secret string) string {
	t.Helper()
	now := time.Now()
	claims := auth.JWTClaims{
		UserID:         "",
		OrganizationID: orgID,
		Role:           role,
		Email:          email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   "",
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("makeEmptyUserIDToken: %v", err)
	}
	return tok
}

// makeEmptyEmailToken generates a correctly-signed HS256 JWT with email set
// to the empty string. ParseToken's explicit claims check
// (`claims.Email == "" → ErrInvalidToken`) must reject it. If that check is
// removed, the token passes to the Me handler, which finds the user by user_id
// and returns 200 — making tests that assert 401 catch the regression.
func makeEmptyEmailToken(t testing.TB, userID, orgID, role, secret string) string {
	t.Helper()
	now := time.Now()
	claims := auth.JWTClaims{
		UserID:         userID,
		OrganizationID: orgID,
		Role:           role,
		Email:          "",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("makeEmptyEmailToken: %v", err)
	}
	return tok
}

// makeWrongIssuerToken generates a correctly-signed HS256 token with a
// non-"playarena" issuer claim. ValidateToken must reject it.
func makeWrongIssuerToken(t testing.TB, userID, orgID, role, email, secret string) string {
	t.Helper()
	now := time.Now()
	claims := auth.JWTClaims{
		UserID:         userID,
		OrganizationID: orgID,
		Role:           role,
		Email:          email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "evil-issuer",
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("makeWrongIssuerToken: %v", err)
	}
	return tok
}

// uniqueUser returns a unique email, username, and full name for use in
// registration tests that call the real registration endpoint.
func uniqueUser(t testing.TB) (email, username, fullName string) {
	t.Helper()
	// Use nanoseconds to guarantee uniqueness across parallel test runs.
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	return "regtest_" + suffix + "@example.com",
		"reguser" + suffix[:12],
		"Reg User"
}
