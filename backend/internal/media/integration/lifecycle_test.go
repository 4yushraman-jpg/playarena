package media_integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// ── response structs ──────────────────────────────────────────────────────────

type mediaResponse struct {
	ID         string  `json:"id"`
	EntityType string  `json:"entity_type"`
	EntityID   string  `json:"entity_id"`
	MediaType  string  `json:"media_type"`
	FileName   string  `json:"file_name"`
	FileURL    string  `json:"file_url"`
	IsPrimary  bool    `json:"is_primary"`
	AltText    *string `json:"alt_text"`
	CreatedAt  string  `json:"created_at"`
}

type mediaListResponse struct {
	Attachments []mediaResponse `json:"attachments"`
	Total       int64           `json:"total"`
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
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.url+"/api/v1/auth/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("loginAs POST: %v", err)
	}
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

// whitePNG returns a valid 1×1 white RGB PNG.
func whitePNG() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x10, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x62, 0xfa, 0xff, 0xff, 0x3f,
		0x20, 0x00, 0x00, 0xff, 0xff, 0x06, 0x06, 0x03,
		0x00, 0xb7, 0x66, 0x11, 0x21, 0x00, 0x00, 0x00,
		0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60,
		0x82,
	}
}

// blackPNG returns a valid 1×1 black RGB PNG (distinct content hash from whitePNG).
func blackPNG() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x10, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x62, 0x62, 0x60, 0x60, 0x00,
		0x04, 0x00, 0x00, 0xff, 0xff, 0x00, 0x0c, 0x00,
		0x03, 0x71, 0x91, 0x8b, 0x17, 0x00, 0x00, 0x00,
		0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60,
		0x82,
	}
}

// ── URL builders ──────────────────────────────────────────────────────────────

func mediaURL(orgSlug string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/media", orgSlug)
}

func mediaItemURL(orgSlug, mediaID string) string {
	return fmt.Sprintf("/api/v1/organizations/%s/media/%s", orgSlug, mediaID)
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

// mustUUID parses a UUID string into pgtype.UUID; fails the test on error.
func mustUUID(t testing.TB, s string) pgtype.UUID {
	t.Helper()
	uid, err := pgutil.ParseUUID(s)
	if err != nil {
		t.Fatalf("mustUUID %q: %v", s, err)
	}
	return uid
}

func uploadImage(t testing.TB, ts *testServer, path, entityType, entityID, token string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	_ = w.WriteField("entity_type", entityType)
	_ = w.WriteField("entity_id", entityID)

	fw, err := w.CreateFormFile("file", "test.png")
	if err != nil {
		t.Fatalf("uploadImage: CreateFormFile: %v", err)
	}
	_, _ = fw.Write(whitePNG())
	w.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.url+path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("uploadImage POST %s: %v", path, err)
	}
	return resp
}

// uploadImageWithPrimary is like uploadImage but sets is_primary=true in the form.
// pngData is the raw PNG bytes to upload; pass nil to use the default white 1×1 PNG.
func uploadImageWithPrimary(t testing.TB, ts *testServer, path, entityType, entityID, token string, pngData []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	_ = w.WriteField("entity_type", entityType)
	_ = w.WriteField("entity_id", entityID)
	_ = w.WriteField("is_primary", "true")

	fw, err := w.CreateFormFile("file", "test.png")
	if err != nil {
		t.Fatalf("uploadImageWithPrimary: CreateFormFile: %v", err)
	}
	if pngData == nil {
		pngData = whitePNG()
	}
	_, _ = fw.Write(pngData)
	w.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.url+path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("uploadImageWithPrimary POST %s: %v", path, err)
	}
	return resp
}

func getReq(t testing.TB, ts *testServer, path string, headers map[string]string) *http.Response {
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

func patchReq(t testing.TB, ts *testServer, path string, body any, headers map[string]string) *http.Response {
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

func deleteReq(t testing.TB, ts *testServer, path string, headers map[string]string) *http.Response {
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

// ── tests ─────────────────────────────────────────────────────────────────────

// TestMedia_Upload_Success verifies POST /media returns 201 for a valid PNG upload
// attached to an organization entity.
func TestMedia_Upload_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := uploadImage(t, ts, mediaURL(actor.orgSlug), "organization", actor.orgID, actor.token)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	var got mediaResponse
	decodeBody(t, resp, &got)
	if got.ID == "" {
		t.Error("expected media ID in response")
	}
	if got.EntityType != "organization" {
		t.Errorf("entity_type = %q, want organization", got.EntityType)
	}
	if got.FileURL == "" {
		t.Error("expected non-empty file_url")
	}
}

// TestMedia_List_Default verifies GET /media returns a paginated list after upload.
func TestMedia_List_Default(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	// Upload one attachment.
	up := uploadImage(t, ts, mediaURL(actor.orgSlug), "organization", actor.orgID, actor.token)
	up.Body.Close()

	resp := getReq(t, ts, mediaURL(actor.orgSlug), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list mediaListResponse
	decodeBody(t, resp, &list)
	if list.Total < 1 {
		t.Errorf("total = %d, want >= 1 after upload", list.Total)
	}
}

// TestMedia_GetByID_Success verifies GET /media/{id} returns the attachment.
func TestMedia_GetByID_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	up := uploadImage(t, ts, mediaURL(actor.orgSlug), "organization", actor.orgID, actor.token)
	defer up.Body.Close()
	assertStatus(t, up, http.StatusCreated)
	var uploaded mediaResponse
	decodeBody(t, up, &uploaded)

	resp := getReq(t, ts, mediaItemURL(actor.orgSlug, uploaded.ID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got mediaResponse
	decodeBody(t, resp, &got)
	if got.ID != uploaded.ID {
		t.Errorf("id = %q, want %q", got.ID, uploaded.ID)
	}
}

// TestMedia_Update_AltText verifies PATCH /media/{id} updates alt_text.
func TestMedia_Update_AltText(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	up := uploadImage(t, ts, mediaURL(actor.orgSlug), "organization", actor.orgID, actor.token)
	defer up.Body.Close()
	assertStatus(t, up, http.StatusCreated)
	var uploaded mediaResponse
	decodeBody(t, up, &uploaded)

	altText := "Test alt text"
	resp := patchReq(t, ts, mediaItemURL(actor.orgSlug, uploaded.ID), map[string]any{
		"alt_text": altText,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got mediaResponse
	decodeBody(t, resp, &got)
	if got.AltText == nil || *got.AltText != altText {
		t.Errorf("alt_text = %v, want %q", got.AltText, altText)
	}
}

// TestMedia_Delete_Success verifies DELETE /media/{id} removes the attachment.
func TestMedia_Delete_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	up := uploadImage(t, ts, mediaURL(actor.orgSlug), "organization", actor.orgID, actor.token)
	defer up.Body.Close()
	assertStatus(t, up, http.StatusCreated)
	var uploaded mediaResponse
	decodeBody(t, up, &uploaded)

	del := deleteReq(t, ts, mediaItemURL(actor.orgSlug, uploaded.ID), bearerHeader(actor.token))
	defer del.Body.Close()
	assertStatus(t, del, http.StatusNoContent)

	// Confirm it's gone.
	get := getReq(t, ts, mediaItemURL(actor.orgSlug, uploaded.ID), bearerHeader(actor.token))
	defer get.Body.Close()
	assertStatus(t, get, http.StatusNotFound)
}

// TestMedia_Upload_NoAuth verifies POST /media without a token returns 401.
func TestMedia_Upload_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	resp := uploadImage(t, ts, mediaURL(actor.orgSlug), "organization", actor.orgID, "")
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

// TestMedia_Upload_WrongOrg_BOLA verifies an actor from Org A cannot upload
// media for Org B.
func TestMedia_Upload_WrongOrg_BOLA(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	// Org A actor uploads to Org B's URL.
	resp := uploadImage(t, ts, mediaURL(orgB.orgSlug), "organization", orgB.orgID, orgA.token)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestMedia_Upload_InvalidEntityType verifies an unsupported entity_type returns 400.
func TestMedia_Upload_InvalidEntityType(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("entity_type", "match")
	_ = mw.WriteField("entity_id", actor.orgID)
	fw, _ := mw.CreateFormFile("file", "test.png")
	_, _ = fw.Write([]byte("fake"))
	mw.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.url+mediaURL(actor.orgSlug), &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+actor.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for unsupported entity_type, got %d", resp.StatusCode)
	}
}

// TestMedia_List_OrgScoped verifies that media from Org B is not visible when listing
// Org A's media.
func TestMedia_List_OrgScoped(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	// Upload to Org B.
	up := uploadImage(t, ts, mediaURL(orgB.orgSlug), "organization", orgB.orgID, orgB.token)
	defer up.Body.Close()
	assertStatus(t, up, http.StatusCreated)
	var uploaded mediaResponse
	decodeBody(t, up, &uploaded)

	// List Org A's media.
	resp := getReq(t, ts, mediaURL(orgA.orgSlug), bearerHeader(orgA.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var list mediaListResponse
	decodeBody(t, resp, &list)
	for _, m := range list.Attachments {
		if m.ID == uploaded.ID {
			t.Errorf("Org B media %q appeared in Org A's list", uploaded.ID)
		}
	}
}

// TestMedia_Upload_NoPermission verifies that a same-org viewer cannot upload media.
// The viewer is in the same org as the owner so 403 comes from permission denial,
// not from BOLA.
func TestMedia_Upload_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	ownerCtx := setupUserAndOrg(t, ts, "org_owner")
	orgUID := mustUUID(t, ownerCtx.orgID)

	viewerUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	fixtures.AddUserToOrg(ctx, t, ts.pool, orgUID, viewerUser.ID, "viewer")
	viewerToken := loginAs(t, ts, viewerUser.Email, fixtures.KnownPasswordRaw, ownerCtx.orgID)

	resp := uploadImage(t, ts, mediaURL(ownerCtx.orgSlug), "organization", ownerCtx.orgID, viewerToken)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestMedia_PrimarySwap verifies that uploading a second attachment with
// is_primary=true atomically demotes the existing primary to is_primary=false.
func TestMedia_PrimarySwap(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	// Upload A with is_primary=true — A becomes the primary.
	upA := uploadImageWithPrimary(t, ts, mediaURL(actor.orgSlug), "organization", actor.orgID, actor.token, whitePNG())
	defer upA.Body.Close()
	assertStatus(t, upA, http.StatusCreated)
	var mediaA mediaResponse
	decodeBody(t, upA, &mediaA)
	if !mediaA.IsPrimary {
		t.Error("media A should be primary after upload with is_primary=true")
	}

	// Upload B with is_primary=true — B becomes primary, A is demoted.
	// Use a distinct image (black pixel) so duplicate detection doesn't short-circuit the swap.
	upB := uploadImageWithPrimary(t, ts, mediaURL(actor.orgSlug), "organization", actor.orgID, actor.token, blackPNG())
	defer upB.Body.Close()
	assertStatus(t, upB, http.StatusCreated)
	var mediaB mediaResponse
	decodeBody(t, upB, &mediaB)
	if !mediaB.IsPrimary {
		t.Error("media B should be primary after upload with is_primary=true")
	}

	// Fetch A and verify it was demoted.
	respA := getReq(t, ts, mediaItemURL(actor.orgSlug, mediaA.ID), bearerHeader(actor.token))
	defer respA.Body.Close()
	assertStatus(t, respA, http.StatusOK)
	var gotA mediaResponse
	decodeBody(t, respA, &gotA)
	if gotA.IsPrimary {
		t.Error("media A should no longer be primary after media B was uploaded as primary")
	}
}
