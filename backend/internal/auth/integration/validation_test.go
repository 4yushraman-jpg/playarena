package auth_integration_test

import (
	"strings"
	"testing"
)

// ---- Login validation ---------------------------------------------------------

// TestLogin_InvalidEmailFormat verifies that a login request with an invalid
// email format returns 400 with the ValidationError fields response shape.
//
// Regression gate: if the email validator rule is removed from LoginRequest,
// the request reaches the service. GetUserByEmail fails (email not found) →
// ErrInvalidCredentials → 401. assertStatus(400) catches the regression and
// confirms the validator fires before any DB access.
func TestLogin_InvalidEmailFormat(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    "not-an-email",
		"password": "Password1!",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertValidationError(t, resp, "email")
}

// TestLogin_PasswordTooShort verifies that a login request with a password
// shorter than the minimum 8 characters returns 400 (ValidationError, min=8
// rule) before any credential check runs.
func TestLogin_PasswordTooShort(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":    "test@example.com",
		"password": "short",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertValidationError(t, resp, "password")
}

// TestLogin_NonUUIDOrgID verifies that supplying a non-UUID string for
// organization_id returns 400 (ValidationError, omitempty,uuid rule) before
// any service or DB access.
//
// Regression gate: if the uuid validator rule is removed, the request reaches
// service.resolveExplicitOrg, which calls pgutil.ParseUUID("not-a-uuid"),
// fails, and returns ErrOrganizationNotFound → 422. assertStatus(400) catches
// the regression.
func TestLogin_NonUUIDOrgID(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email":           "test@example.com",
		"password":        "Password1!",
		"organization_id": "not-a-uuid",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertValidationError(t, resp, "organization_id")
}

// TestLogin_MalformedJSON verifies that a non-JSON body to POST /login returns
// 400 with a plain error message body — not the ValidationError "fields" shape.
//
// This confirms that writeDecodeError has two distinct output paths:
//   - ValidationError (field rules failed) → {"error":"validation failed","fields":{...}}
//   - Other decode error (bad JSON)        → {"error":"..."}
//
// Both return 400 but with different response shapes.
func TestLogin_MalformedJSON(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.postRaw(t, "/api/v1/auth/login", `{not valid json`)
	defer resp.Body.Close()
	assertStatus(t, resp, 400)

	var body struct {
		Error  string            `json:"error"`
		Fields map[string]string `json:"fields"`
	}
	decodeBody(t, resp, &body)
	if body.Error == "" {
		t.Error("malformed JSON: expected non-empty error message")
	}
	if body.Error == "validation failed" {
		t.Errorf("malformed JSON: got ValidationError body; expected plain decode error; body.error = %q", body.Error)
	}
	if len(body.Fields) > 0 {
		t.Errorf("malformed JSON: expected no fields; got %v", body.Fields)
	}
}

// TestLogin_MissingPassword verifies that a login request with the password
// field absent entirely returns 400 with fields.password populated. Tests the
// required rule for a missing field (zero value) vs an explicitly short string.
func TestLogin_MissingPassword(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email": "test@example.com",
		// password field deliberately omitted — zero value triggers required
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertValidationError(t, resp, "password")
}

// ---- Register validation ------------------------------------------------------

// TestRegister_InvalidEmailFormat verifies that registration with an invalid
// email format returns 400 with fields.email populated. The validator fires
// before service.Register is called, so no DB write occurs.
func TestRegister_InvalidEmailFormat(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/register", map[string]any{
		"email":     "not-an-email",
		"password":  "Password1!",
		"username":  "testuser",
		"full_name": "Test User",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertValidationError(t, resp, "email")
}

// TestRegister_UsernameTooShort verifies that a username shorter than 3
// characters returns 400 with fields.username populated (min=3 rule).
func TestRegister_UsernameTooShort(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/register", map[string]any{
		"email":     "test@example.com",
		"password":  "Password1!",
		"username":  "ab", // min=3
		"full_name": "Test User",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertValidationError(t, resp, "username")
}

// TestRegister_UsernameInvalidChars verifies that a username containing
// characters outside [a-zA-Z0-9_] returns 400 (alphanum_under rule).
//
// This rule mirrors the DB CHECK constraint on users.username. Regression
// gate: if alphanum_under is removed from RegisterRequest, the request reaches
// service.Register which writes the raw username to the DB. The DB CHECK fires
// and returns a 500 "internal server error". assertStatus(400) catches the
// regression before any DB write occurs.
func TestRegister_UsernameInvalidChars(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/register", map[string]any{
		"email":     "test@example.com",
		"password":  "Password1!",
		"username":  "user-name!", // hyphen and ! violate alphanum_under
		"full_name": "Test User",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertValidationError(t, resp, "username")
}

// TestRegister_PasswordTooShort verifies that a password shorter than 8 chars
// returns 400 from the validator before service.Register is called.
func TestRegister_PasswordTooShort(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/register", map[string]any{
		"email":     "test@example.com",
		"password":  "short",
		"username":  "testuser",
		"full_name": "Test User",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertValidationError(t, resp, "password")
}

// TestRegister_FullNameEmpty verifies that an empty full_name field returns 400
// with fields.full_name populated (required rule).
func TestRegister_FullNameEmpty(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/register", map[string]any{
		"email":     "test@example.com",
		"password":  "Password1!",
		"username":  "testuser",
		"full_name": "",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertValidationError(t, resp, "full_name")
}

// TestRegister_PasswordTooLongBytes verifies that a password exceeding 72 UTF-8
// bytes returns 422 "password exceeds maximum length" even when the rune count
// is ≤ 72.
//
// The RegisterRequest validator enforces max=72 by rune count (using
// len([]rune(value)) in the validator). HashPassword enforces 72 by byte count
// (using len([]byte(password))). For ASCII these are equivalent. For multi-byte
// Unicode they diverge: 37 × 'é' (U+00E9, 2 UTF-8 bytes each) is 37 runes but
// 74 bytes, passing the rune-count validator but failing the byte-count check.
//
// This is the only production path that produces ErrPasswordTooLong → 422.
// HashPassword's byte check fires before any DB write.
func TestRegister_PasswordTooLongBytes(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	// 37 × 'é' = 37 runes (≤ 72: passes max=72 rune validator),
	// 74 bytes (> 72: fails HashPassword byte-count check → ErrPasswordTooLong).
	longPass := strings.Repeat("é", 37)

	resp := ts.post(t, "/api/v1/auth/register", map[string]any{
		"email":     "pwlong@example.com",
		"password":  longPass,
		"username":  "pwlonguser",
		"full_name": "Test User",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 422)
	assertErrorBody(t, resp, "password exceeds maximum length")
}

// ---- Refresh validation -------------------------------------------------------

// TestRefresh_EmptyRefreshToken verifies that an empty string for refresh_token
// returns 400 (ValidationError, required rule). This is distinct from
// TestRefresh_InvalidToken which uses a non-empty garbage string and returns 401
// (service layer: hash not found in DB). These are different code paths.
func TestRefresh_EmptyRefreshToken(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/refresh", map[string]any{
		"refresh_token": "",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertValidationError(t, resp, "refresh_token")
}

// TestRefresh_NonUUIDOrgID verifies that a non-UUID value for organization_id
// in a refresh request returns 400 before the refresh token is validated.
func TestRefresh_NonUUIDOrgID(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/refresh", map[string]any{
		"refresh_token":   "any-non-empty-value",
		"organization_id": "not-a-uuid",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertValidationError(t, resp, "organization_id")
}

// TestRefresh_MissingBody verifies that an empty POST body to /refresh returns
// 400 with a plain error (not a ValidationError "fields" response). An empty
// body causes DecodeJSON to receive io.EOF, which it maps to
// "request body is required" — a non-ValidationError path through
// writeDecodeError.
func TestRefresh_MissingBody(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.postRaw(t, "/api/v1/auth/refresh", "")
	defer resp.Body.Close()
	assertStatus(t, resp, 400)

	var body struct {
		Error  string            `json:"error"`
		Fields map[string]string `json:"fields"`
	}
	decodeBody(t, resp, &body)
	if body.Error == "" {
		t.Error("missing body: expected non-empty error message")
	}
	if body.Error == "validation failed" {
		t.Errorf("missing body: got ValidationError; expected plain decode error; body.error = %q", body.Error)
	}
	if len(body.Fields) > 0 {
		t.Errorf("missing body: expected no fields; got %v", body.Fields)
	}
}

// ---- ForgotPassword validation ------------------------------------------------

// TestForgotPassword_InvalidEmailFormat verifies that a forgot-password request
// with an invalid email format returns 400 — NOT 200.
//
// Critical boundary: the "always returns 200" contract is service-level only.
// When the validator rejects the request (before the service is called), the
// handler calls writeDecodeError → 400. This test establishes and regression-
// gates that boundary. A future refactor that moved the validator guard below
// the service call would break it — this test catches that.
func TestForgotPassword_InvalidEmailFormat(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/forgot-password", map[string]any{
		"email": "not-an-email",
	})
	defer resp.Body.Close()
	// 400 — NOT 200. The always-200 guarantee applies only to service-level
	// outcomes (unknown email, internal errors), not to validator rejections.
	assertStatus(t, resp, 400)
	assertValidationError(t, resp, "email")
}

// TestForgotPassword_MalformedJSON verifies that a malformed JSON body to
// POST /forgot-password returns 400 — NOT 200. Same boundary as
// TestForgotPassword_InvalidEmailFormat but via the JSON decode path.
func TestForgotPassword_MalformedJSON(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.postRaw(t, "/api/v1/auth/forgot-password", `{not valid json`)
	defer resp.Body.Close()
	// 400 — NOT 200.
	assertStatus(t, resp, 400)

	var body struct {
		Error  string            `json:"error"`
		Fields map[string]string `json:"fields"`
	}
	decodeBody(t, resp, &body)
	if body.Error == "" {
		t.Error("malformed JSON: expected non-empty error message")
	}
	if body.Error == "validation failed" {
		t.Errorf("malformed JSON: got ValidationError; expected plain decode error; body.error = %q", body.Error)
	}
	if len(body.Fields) > 0 {
		t.Errorf("malformed JSON: expected no fields; got %v", body.Fields)
	}
}

// ---- ResetPassword validation -------------------------------------------------

// TestResetPassword_PasswordTooShort verifies that a reset-password request
// with a password shorter than 8 characters returns 400 from the validator.
// The token is NOT consumed — the validator fires before any service call.
func TestResetPassword_PasswordTooShort(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.post(t, "/api/v1/auth/reset-password", map[string]any{
		"token":    "any-non-empty-token",
		"password": "short",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 400)
	assertValidationError(t, resp, "password")
}

// TestResetPassword_PasswordTooLongBytes verifies the same ErrPasswordTooLong
// path via the reset-password endpoint. HashPassword is called before any token
// validation or DB access, so no valid reset token is required.
//
// See TestRegister_PasswordTooLongBytes for the rune-count vs byte-count
// boundary explanation.
func TestResetPassword_PasswordTooLongBytes(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	longPass := strings.Repeat("é", 37)

	resp := ts.post(t, "/api/v1/auth/reset-password", map[string]any{
		"token":    "any-non-empty-token",
		"password": longPass,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 422)
	assertErrorBody(t, resp, "password exceeds maximum length")
}
