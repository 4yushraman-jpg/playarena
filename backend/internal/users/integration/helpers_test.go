package users_integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// ── response types ────────────────────────────────────────────────────────────

type userResp struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	Username    string  `json:"username"`
	FullName    string  `json:"full_name"`
	Status      string  `json:"status"`
	Phone       *string `json:"phone"`
	DateOfBirth *string `json:"date_of_birth"`
	Gender      *string `json:"gender"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type listResp struct {
	Users  []userResp `json:"users"`
	Total  int64      `json:"total"`
	Limit  int32      `json:"limit"`
	Offset int32      `json:"offset"`
}

type errResp struct {
	Error string `json:"error"`
}

type messageResp struct {
	Message string `json:"message"`
}

type badRequestResp struct {
	Error  string `json:"error"`
	Field  string `json:"field"`
	Detail string `json:"detail"`
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

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

func (ts *testServer) post(t testing.TB, path string, body any) *http.Response {
	t.Helper()
	return ts.do(t, http.MethodPost, path, body, nil)
}

func (ts *testServer) patch(t testing.TB, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	return ts.do(t, http.MethodPatch, path, body, headers)
}

func (ts *testServer) postWithHeaders(t testing.TB, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	return ts.do(t, http.MethodPost, path, body, headers)
}

func (ts *testServer) do(t testing.TB, method, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("helpers: marshal body: %v", err)
	}
	req, err := http.NewRequest(method, ts.url+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("helpers: build %s request: %v", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("helpers: %s %s: %v", method, path, err)
	}
	return resp
}

// ── assertion helpers ─────────────────────────────────────────────────────────

func assertStatus(t testing.TB, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected HTTP %d, got %d; body: %s", want, resp.StatusCode, body)
	}
}

func decodeBody(t testing.TB, resp *http.Response, dest any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		t.Fatalf("helpers: decode body: %v", err)
	}
}

func assertErrorBody(t testing.TB, resp *http.Response, wantMsg string) {
	t.Helper()
	defer resp.Body.Close()
	var e errResp
	decodeBody(t, resp, &e)
	if e.Error != wantMsg {
		t.Errorf("error body: got %q, want %q", e.Error, wantMsg)
	}
}

func bearerHeader(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

// doPostWithHeaders is a goroutine-safe POST helper that returns the status
// code and raw response body. It does not call t.Fatal, making it safe to
// invoke from goroutines in concurrency tests.
func doPostWithHeaders(ts *testServer, path string, body any, headers map[string]string) (int, string, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return 0, "", err
	}
	req, err := http.NewRequest(http.MethodPost, ts.url+path, bytes.NewReader(b))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, string(bodyBytes), nil
}

// doPatchWithHeaders is a goroutine-safe PATCH helper. It does not call
// t.Fatal, making it safe to invoke from goroutines in concurrency tests.
func doPatchWithHeaders(ts *testServer, path string, body any, headers map[string]string) (int, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPatch, ts.url+path, bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}
