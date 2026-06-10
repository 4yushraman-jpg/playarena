package members_integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── response types ────────────────────────────────────────────────────────────

type roleGrant struct {
	GrantID   string  `json:"grant_id"`
	RoleSlug  string  `json:"role_slug"`
	RoleName  string  `json:"role_name"`
	GrantedAt string  `json:"granted_at"`
	ExpiresAt *string `json:"expires_at"`
	GrantedBy *string `json:"granted_by"`
}

type memberResponse struct {
	UserID     string      `json:"user_id"`
	Email      string      `json:"email"`
	Username   string      `json:"username"`
	UserStatus string      `json:"user_status"`
	Roles      []roleGrant `json:"roles"`
}

type listResponse struct {
	Members []memberResponse `json:"members"`
}

type errResp struct {
	Error string `json:"error"`
}

// ── token helpers ─────────────────────────────────────────────────────────────

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

type orgContext struct {
	token   string
	orgID   string
	orgSlug string
}

func setupUserAndOrg(t testing.TB, ts *testServer, roleSlug string) orgContext {
	t.Helper()
	ctx := context.Background()

	user := fixtures.CreateActiveUser(ctx, t, ts.pool)
	org := fixtures.CreateOrgForUser(ctx, t, ts.pool, user.ID, roleSlug)
	orgIDStr := pgutil.UUIDToString(org.ID)

	token := loginAs(t, ts, user.Email, fixtures.KnownPasswordRaw, orgIDStr)
	return orgContext{token: token, orgID: orgIDStr, orgSlug: org.Slug}
}

// ── URL helpers ───────────────────────────────────────────────────────────────

func membersURL(orgSlug string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/members", orgSlug)
}

func memberURL(orgSlug, userID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/members/%s", orgSlug, userID)
}

func grantURL(orgSlug, userID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/members/%s/roles", orgSlug, userID)
}

func revokeURL(orgSlug, userID, roleSlug string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/members/%s/roles/%s", orgSlug, userID, roleSlug)
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func (ts *testServer) post(t testing.TB, path string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func (ts *testServer) postWithToken(t testing.TB, path string, body any, token string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func (ts *testServer) getWithToken(t testing.TB, path, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.url+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func (ts *testServer) deleteWithToken(t testing.TB, path, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, ts.url+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	return resp
}

// ── assertions ────────────────────────────────────────────────────────────────

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

func hasRole(member memberResponse, slug string) bool {
	for _, g := range member.Roles {
		if g.RoleSlug == slug {
			return true
		}
	}
	return false
}
