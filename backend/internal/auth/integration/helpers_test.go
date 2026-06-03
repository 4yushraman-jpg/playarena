package auth_integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// ---- response structs -------------------------------------------------------

type loginResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type registerResp struct {
	ID                string `json:"id"`
	Email             string `json:"email"`
	Username          string `json:"username"`
	Message           string `json:"message"`
	VerificationToken string `json:"verification_token"`
}

type forgotPasswordResp struct {
	Message    string `json:"message"`
	ResetToken string `json:"reset_token"`
}

type meResp struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	Username       string `json:"username"`
	FullName       string `json:"full_name"`
	Status         string `json:"status"`
	Role           string `json:"role"`
	OrganizationID string `json:"organization_id"`
}

type errResp struct {
	Error string `json:"error"`
}

type orgRequiredResp struct {
	Error         string `json:"error"`
	Code          string `json:"code"`
	Organizations []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"organizations"`
}

type messageResp struct {
	Message string `json:"message"`
}

// ---- HTTP client helpers ----------------------------------------------------

// post sends a JSON POST request to ts.url+path. The caller must close
// resp.Body. Uses t.Fatal if the request cannot be executed.
func (ts *testServer) post(t testing.TB, path string, body any) *http.Response {
	t.Helper()
	return ts.postWithHeaders(t, path, body, nil)
}

// postWithHeaders sends a JSON POST request with additional headers.
// The caller must close resp.Body.
func (ts *testServer) postWithHeaders(t testing.TB, path string, body any, headers map[string]string) *http.Response {
	t.Helper()

	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("helpers: marshal request body: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.url+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("helpers: build POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("helpers: POST %s: %v", path, err)
	}
	return resp
}

// get sends a GET request to ts.url+path with optional headers.
// The caller must close resp.Body.
func (ts *testServer) get(t testing.TB, path string, headers map[string]string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, ts.url+path, nil)
	if err != nil {
		t.Fatalf("helpers: build GET request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("helpers: GET %s: %v", path, err)
	}
	return resp
}

// options sends an OPTIONS request to ts.url+path with optional headers.
// The caller must close resp.Body.
func (ts *testServer) options(t testing.TB, path string, headers map[string]string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodOptions, ts.url+path, nil)
	if err != nil {
		t.Fatalf("helpers: build OPTIONS request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("helpers: OPTIONS %s: %v", path, err)
	}
	return resp
}

// doPost is the concurrency-safe variant of post: it does not call t.Fatal
// and returns an error instead. Intended for goroutine use in concurrency tests.
func doPost(ts *testServer, path string, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.url+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

// doGet is the concurrency-safe variant of get.
func doGet(ts *testServer, path string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, ts.url+path, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return http.DefaultClient.Do(req)
}

// ---- assertion helpers ------------------------------------------------------

// assertStatus asserts resp.StatusCode == want. On mismatch it prints the body.
func assertStatus(t testing.TB, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected HTTP %d, got %d; body: %s", want, resp.StatusCode, body)
	}
}

// decodeBody decodes the response body into dest using JSON.
// It does NOT close the body; callers should defer resp.Body.Close() first.
func decodeBody(t testing.TB, resp *http.Response, dest any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		t.Fatalf("helpers: decode response body: %v", err)
	}
}

// assertErrorBody asserts that the response has a JSON body with
// {"error": wantMsg}. It also closes resp.Body.
func assertErrorBody(t testing.TB, resp *http.Response, wantMsg string) {
	t.Helper()
	defer resp.Body.Close()
	var e errResp
	decodeBody(t, resp, &e)
	if e.Error != wantMsg {
		t.Errorf("error body: got %q, want %q", e.Error, wantMsg)
	}
}

// bearerHeader returns an Authorization header map for the given access token.
func bearerHeader(accessToken string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + accessToken}
}
