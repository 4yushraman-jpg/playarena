package auth_integration_test

import (
	"testing"
)

// Rate-limit tests use dedicated per-test servers with a restrictive limiter
// (burst=1). Each test gets its own fresh IPRateLimiter instance so tests
// cannot interfere with each other's buckets.
//
// With burst=1 and RPS=1 the first request always consumes the single
// token in the bucket; the second request immediately after will be
// rate-limited. HTTP round-trip latency (~1 ms) is orders of magnitude
// shorter than the 1-second refill window, making exhaustion deterministic.

// TestRateLimit_LoginExhaustion verifies that the second rapid POST to
// /api/v1/auth/login returns 429 after the burst bucket is exhausted.
func TestRateLimit_LoginExhaustion(t *testing.T) {
	ts := buildRateLimitedTestServer(t, testPool, 1, 1)

	resp1 := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email": "rate@example.com", "password": "Password1!",
	})
	resp1.Body.Close()

	resp2 := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email": "rate@example.com", "password": "Password1!",
	})
	defer resp2.Body.Close()
	assertStatus(t, resp2, 429)
	assertErrorBody(t, resp2, "rate limit exceeded")
}

// TestRateLimit_RegisterExhaustion verifies that the second rapid POST to
// /api/v1/auth/register returns 429.
func TestRateLimit_RegisterExhaustion(t *testing.T) {
	ts := buildRateLimitedTestServer(t, testPool, 1, 1)

	resp1 := ts.post(t, "/api/v1/auth/register", map[string]any{
		"email": "ratelimit_r@example.com", "password": "Password1!",
		"username": "ratelimitreg", "full_name": "Rate User",
	})
	resp1.Body.Close()

	resp2 := ts.post(t, "/api/v1/auth/register", map[string]any{
		"email": "ratelimit_r2@example.com", "password": "Password1!",
		"username": "ratelimitreg2", "full_name": "Rate User2",
	})
	defer resp2.Body.Close()
	assertStatus(t, resp2, 429)
	assertErrorBody(t, resp2, "rate limit exceeded")
}

// TestRateLimit_RefreshExhaustion verifies that the second rapid POST to
// /api/v1/auth/refresh returns 429.
func TestRateLimit_RefreshExhaustion(t *testing.T) {
	ts := buildRateLimitedTestServer(t, testPool, 1, 1)

	resp1 := ts.post(t, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": "dummy",
	})
	resp1.Body.Close()

	resp2 := ts.post(t, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": "dummy",
	})
	defer resp2.Body.Close()
	assertStatus(t, resp2, 429)
	assertErrorBody(t, resp2, "rate limit exceeded")
}

// TestRateLimit_ForgotPasswordExhaustion verifies that the second rapid POST to
// /api/v1/auth/forgot-password returns 429.
func TestRateLimit_ForgotPasswordExhaustion(t *testing.T) {
	ts := buildRateLimitedTestServer(t, testPool, 1, 1)

	resp1 := ts.post(t, "/api/v1/auth/forgot-password", map[string]string{
		"email": "nobody@example.com",
	})
	resp1.Body.Close()

	resp2 := ts.post(t, "/api/v1/auth/forgot-password", map[string]string{
		"email": "nobody@example.com",
	})
	defer resp2.Body.Close()
	assertStatus(t, resp2, 429)
	assertErrorBody(t, resp2, "rate limit exceeded")
}

// TestRateLimit_AllAuthRoutesSameBucket verifies that all /api/v1/auth routes
// share the same per-IP bucket. After one request exhausts the burst, a
// request to a different endpoint on the same path prefix also receives 429.
func TestRateLimit_AllAuthRoutesSameBucket(t *testing.T) {
	ts := buildRateLimitedTestServer(t, testPool, 1, 1)

	// Consume the single token with a login request.
	resp1 := ts.post(t, "/api/v1/auth/login", map[string]any{
		"email": "rate@example.com", "password": "Password1!",
	})
	resp1.Body.Close()

	// A register request to the same route group must also be rate-limited.
	resp2 := ts.post(t, "/api/v1/auth/register", map[string]any{
		"email": "ratelimitshared@example.com", "password": "Password1!",
		"username": "ratelimitshared", "full_name": "Shared Rate",
	})
	defer resp2.Body.Close()
	assertStatus(t, resp2, 429)
	assertErrorBody(t, resp2, "rate limit exceeded")
}
