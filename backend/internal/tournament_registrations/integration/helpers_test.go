package tournament_registrations_integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── response structs ──────────────────────────────────────────────────────────

type registrationResponse struct {
	ID             string  `json:"id"`
	TournamentID   string  `json:"tournament_id"`
	OrganizationID string  `json:"organization_id"`
	TeamID         *string `json:"team_id"`
	Status         string  `json:"status"`
	RegisteredAt   string  `json:"registered_at"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

type registrationListResponse struct {
	Registrations []registrationResponse `json:"registrations"`
	Total         int64                  `json:"total"`
	Limit         int                    `json:"limit"`
	Offset        int                    `json:"offset"`
}

type errResp struct {
	Error string `json:"error"`
}

// ── token acquisition ─────────────────────────────────────────────────────────

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

func mustUUID(t testing.TB, s string) pgtype.UUID {
	t.Helper()
	uid, err := pgutil.ParseUUID(s)
	if err != nil {
		t.Fatalf("mustUUID %q: %v", s, err)
	}
	return uid
}

// ── URL builders ──────────────────────────────────────────────────────────────

func registrationsURL(orgSlug, tournamentID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/tournaments/%s/registrations", orgSlug, tournamentID)
}

func registrationURL(orgSlug, tournamentID, registrationID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/tournaments/%s/registrations/%s", orgSlug, tournamentID, registrationID)
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

func (ts *testServer) postWithHeaders(t testing.TB, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func (ts *testServer) get(t testing.TB, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.url+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func (ts *testServer) patch(t testing.TB, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPatch, ts.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", path, err)
	}
	return resp
}

func (ts *testServer) delete(t testing.TB, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, ts.url+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	return resp
}

func (ts *testServer) postRawWithHeaders(t testing.TB, path string, rawBody string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, ts.url+path, strings.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST raw %s: %v", path, err)
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

func bearerHeader(accessToken string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + accessToken}
}
