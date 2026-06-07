package organizations_integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── response structs ──────────────────────────────────────────────────────────

type orgResponse struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Type        string  `json:"type"`
	Status      string  `json:"status"`
	Description *string `json:"description"`
	Country     *string `json:"country"`
	City        *string `json:"city"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type orgListResponse struct {
	Organizations []orgResponse `json:"organizations"`
	Total         int           `json:"total"`
	Limit         int           `json:"limit"`
	Offset        int           `json:"offset"`
}

type errResp struct {
	Error string `json:"error"`
}

// ── token acquisition ─────────────────────────────────────────────────────────

// loginAs logs in with the given credentials and returns a valid access token.
// orgID may be empty for platform admins.
func loginAs(t testing.TB, ts *testServer, emailAddr, password, orgID string) string {
	t.Helper()
	body := map[string]any{"email": emailAddr, "password": password}
	if orgID != "" {
		body["organization_id"] = orgID
	}
	resp := ts.post(t, "/api/v1/auth/login", body)
	defer resp.Body.Close()
	assertStatus(t, resp, 200)
	var r struct {
		AccessToken string `json:"access_token"`
	}
	decodeBody(t, resp, &r)
	if r.AccessToken == "" {
		t.Fatal("loginAs: empty access_token")
	}
	return r.AccessToken
}

// userAndOrg holds the pre-created user+org setup used by most test cases.
type userAndOrg struct {
	token   string
	orgID   string // UUID string
	orgSlug string
}

// setupPlatformAdmin creates a platform_admin user, logs them in (no org
// context), and returns the access token.
func setupPlatformAdmin(t testing.TB, ts *testServer) string {
	t.Helper()
	ctx := context.Background()
	admin := fixtures.CreatePlatformAdmin(ctx, t, ts.pool)
	return loginAs(t, ts, admin.Email, fixtures.KnownPasswordRaw, "")
}

// setupUserAndOrg creates an active user, grants them the given role in a new
// organization, and logs in. Returns a userAndOrg with the access token, org
// UUID, and org slug.
func setupUserAndOrg(t testing.TB, ts *testServer, roleSlug string) userAndOrg {
	t.Helper()
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, ts.pool)
	org := fixtures.CreateOrgForUser(ctx, t, ts.pool, user.ID, roleSlug)
	orgIDStr := pgutil.UUIDToString(org.ID)

	token := loginAs(t, ts, user.Email, fixtures.KnownPasswordRaw, orgIDStr)
	return userAndOrg{token: token, orgID: orgIDStr, orgSlug: org.Slug}
}

// ── HTTP client helpers ───────────────────────────────────────────────────────

func (ts *testServer) post(t testing.TB, path string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("helpers: marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.url+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("helpers: build POST: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("helpers: POST %s: %v", path, err)
	}
	return resp
}

func (ts *testServer) postWithHeaders(t testing.TB, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("helpers: marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.url+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("helpers: build POST: %v", err)
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

func (ts *testServer) get(t testing.TB, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.url+path, nil)
	if err != nil {
		t.Fatalf("helpers: build GET: %v", err)
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

func (ts *testServer) patch(t testing.TB, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("helpers: marshal patch body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPatch, ts.url+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("helpers: build PATCH: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("helpers: PATCH %s: %v", path, err)
	}
	return resp
}

func (ts *testServer) delete(t testing.TB, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, ts.url+path, nil)
	if err != nil {
		t.Fatalf("helpers: build DELETE: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("helpers: DELETE %s: %v", path, err)
	}
	return resp
}

func (ts *testServer) postRaw(t testing.TB, path string, rawBody string) *http.Response {
	t.Helper()
	return ts.postRawWithHeaders(t, path, rawBody, nil)
}

func (ts *testServer) postRawWithHeaders(t testing.TB, path string, rawBody string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.url+path, strings.NewReader(rawBody))
	if err != nil {
		t.Fatalf("helpers: build POST raw: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("helpers: POST raw %s: %v", path, err)
	}
	return resp
}

// doPost is goroutine-safe (does not call t.Fatal). For use in concurrency tests.
func doPost(ts *testServer, path string, body any, headers map[string]string) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.url+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return http.DefaultClient.Do(req)
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
		t.Fatalf("decodeBody: %v", err)
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

func bearerHeader(accessToken string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + accessToken}
}

func assertValidationError(t testing.TB, resp *http.Response, wantField string) {
	t.Helper()
	defer resp.Body.Close()
	var body struct {
		Error  string            `json:"error"`
		Fields map[string]string `json:"fields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("assertValidationError: decode: %v", err)
	}
	if body.Error != "validation failed" {
		t.Errorf("assertValidationError: error = %q, want %q", body.Error, "validation failed")
	}
	if wantField == "" {
		return
	}
	if body.Fields == nil {
		t.Errorf("assertValidationError: fields is nil, want key %q", wantField)
		return
	}
	if msg, ok := body.Fields[wantField]; !ok || msg == "" {
		t.Errorf("assertValidationError: fields[%q] missing or empty; fields = %v", wantField, body.Fields)
	}
}
