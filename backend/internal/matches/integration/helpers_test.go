package matches_integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── response structs ──────────────────────────────────────────────────────────

type matchResponse struct {
	ID             string  `json:"id"`
	TournamentID   string  `json:"tournament_id"`
	OrganizationID string  `json:"organization_id"`
	Status         string  `json:"status"`
	HomeTeamID     *string `json:"home_team_id"`
	AwayTeamID     *string `json:"away_team_id"`
	WinnerTeamID   *string `json:"winner_team_id"`
	IsWalkover     bool    `json:"is_walkover"`
	ScheduledAt    *string `json:"scheduled_at"`
	StartedAt      *string `json:"started_at"`
	EndedAt        *string `json:"ended_at"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

type matchListResponse struct {
	Matches []matchResponse `json:"matches"`
	Total   int64           `json:"total"`
	Limit   int             `json:"limit"`
	Offset  int             `json:"offset"`
}

type scoreResponse struct {
	MatchID   string `json:"match_id"`
	HomeScore int    `json:"home_score"`
	AwayScore int    `json:"away_score"`
	Status    string `json:"status"`
}

type eventResponse struct {
	ID             string `json:"id"`
	MatchID        string `json:"match_id"`
	SequenceNumber int64  `json:"sequence_number"`
	EventType      string `json:"event_type"`
	CreatedAt      string `json:"created_at"`
}

type eventListResponse struct {
	Events []eventResponse `json:"events"`
	Total  int64           `json:"total"`
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

func matchesURL(orgSlug string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/matches", orgSlug)
}

func matchURL(orgSlug, matchID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/matches/%s", orgSlug, matchID)
}

func matchScoreURL(orgSlug, matchID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/matches/%s/score", orgSlug, matchID)
}

func matchEventsURL(orgSlug, matchID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/matches/%s/events", orgSlug, matchID)
}

func matchEventURL(orgSlug, matchID, eventID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/matches/%s/events/%s", orgSlug, matchID, eventID)
}

// scheduledAt returns an RFC3339 timestamp 1 hour in the future.
func scheduledAt() string {
	return time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
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
