package auth_integration_test

import (
	"testing"
)

// CORS middleware behaviour under test:
//
//	testAllowedOrigin    = "https://allowed.example.com"  → matched
//	testDisallowedOrigin = "https://evil.example.com"     → not matched
//
// The CORS middleware always sets Access-Control-Allow-Methods,
// Access-Control-Allow-Headers, Access-Control-Max-Age, and Vary: Origin.
// Access-Control-Allow-Origin and Access-Control-Allow-Credentials are set
// only when the request origin is in the allowedOrigins list.

// TestCORS_Preflight_AllowedOrigin verifies that an OPTIONS preflight from an
// allowed origin returns 204 with the full CORS header set including
// Access-Control-Allow-Origin and Access-Control-Allow-Credentials: true.
func TestCORS_Preflight_AllowedOrigin(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.options(t, "/api/v1/auth/login", map[string]string{
		"Origin": testAllowedOrigin,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 204)

	got := resp.Header.Get("Access-Control-Allow-Origin")
	if got != testAllowedOrigin {
		t.Errorf("preflight allowed: ACAO got %q, want %q", got, testAllowedOrigin)
	}

	creds := resp.Header.Get("Access-Control-Allow-Credentials")
	if creds != "true" {
		t.Errorf("preflight allowed: ACAC got %q, want %q", creds, "true")
	}

	vary := resp.Header.Get("Vary")
	if vary == "" {
		t.Error("preflight allowed: Vary header absent")
	}

	methods := resp.Header.Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("preflight allowed: Access-Control-Allow-Methods absent")
	}
}

// TestCORS_Preflight_DisallowedOrigin verifies that an OPTIONS preflight from
// a disallowed origin still returns 204 (the method is always intercepted) but
// does NOT include Access-Control-Allow-Origin or Access-Control-Allow-Credentials.
func TestCORS_Preflight_DisallowedOrigin(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.options(t, "/api/v1/auth/login", map[string]string{
		"Origin": testDisallowedOrigin,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, 204)

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("preflight disallowed: ACAO should be absent, got %q", got)
	}

	if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("preflight disallowed: ACAC should be absent, got %q", got)
	}

	// Allow-Methods and Vary are always set regardless of origin match.
	if resp.Header.Get("Access-Control-Allow-Methods") == "" {
		t.Error("preflight disallowed: Access-Control-Allow-Methods should always be set")
	}
	if resp.Header.Get("Vary") == "" {
		t.Error("preflight disallowed: Vary should always be set")
	}
}

// TestCORS_Request_AllowedOrigin verifies that a real (non-preflight) POST
// from an allowed origin receives the ACAO and ACAC headers on the actual
// response (not just on the preflight). The request body is intentionally
// empty to produce a 400, but CORS headers come from the middleware layer
// before the handler runs and are independent of the response status code.
func TestCORS_Request_AllowedOrigin(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.postWithHeaders(t, "/api/v1/auth/login", map[string]any{},
		map[string]string{"Origin": testAllowedOrigin})
	defer resp.Body.Close()
	// The handler returns 400 (validation failure) but the middleware headers
	// are set regardless of outcome.

	got := resp.Header.Get("Access-Control-Allow-Origin")
	if got != testAllowedOrigin {
		t.Errorf("request allowed: ACAO got %q, want %q", got, testAllowedOrigin)
	}

	creds := resp.Header.Get("Access-Control-Allow-Credentials")
	if creds != "true" {
		t.Errorf("request allowed: ACAC got %q, want %q", creds, "true")
	}

	if resp.Header.Get("Vary") == "" {
		t.Error("request allowed: Vary header absent")
	}
}

// TestCORS_Request_DisallowedOrigin verifies that a real POST from a
// disallowed origin does NOT receive Access-Control-Allow-Origin.
func TestCORS_Request_DisallowedOrigin(t *testing.T) {
	t.Parallel()
	ts := buildTestServer(t, testPool)

	resp := ts.postWithHeaders(t, "/api/v1/auth/login", map[string]any{},
		map[string]string{"Origin": testDisallowedOrigin})
	defer resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("request disallowed: ACAO should be absent, got %q", got)
	}

	if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("request disallowed: ACAC should be absent, got %q", got)
	}
}
