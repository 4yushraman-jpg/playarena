package notifications

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/platform/response"
	"github.com/4yushraman-jpg/playarena/internal/platform/validator"
	"github.com/4yushraman-jpg/playarena/internal/realtime"
)

// Handler exposes the notifications service over HTTP.
type Handler struct {
	svc *Service
	hub *realtime.Hub
	cfg *config.Config
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, hub *realtime.Hub, cfg *config.Config, log *slog.Logger) *Handler {
	return &Handler{svc: svc, hub: hub, cfg: cfg, log: log}
}

// ── notification endpoints ────────────────────────────────────────────────────

// List handles GET /api/v1/organizations/{slug}/notifications.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	limit, offset := parsePagination(r)
	resp, err := h.svc.List(r.Context(), slug, principal.UserID, ListParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// GetByID handles GET /api/v1/organizations/{slug}/notifications/{id}.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	resp, err := h.svc.GetByID(r.Context(), slug, id, principal.UserID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// MarkRead handles PATCH /api/v1/organizations/{slug}/notifications/{id}/read.
func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	resp, err := h.svc.MarkRead(r.Context(), slug, id, principal.UserID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// MarkAllRead handles POST /api/v1/organizations/{slug}/notifications/read-all.
func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	if err := h.svc.MarkAllRead(r.Context(), slug, principal.UserID); err != nil {
		h.handleError(w, r, err)
		return
	}
	response.Write(w, http.StatusNoContent, nil)
}

// Delete handles DELETE /api/v1/organizations/{slug}/notifications/{id}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	if err := h.svc.Delete(r.Context(), slug, id, principal.UserID); err != nil {
		h.handleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── preference endpoints ──────────────────────────────────────────────────────

// GetPreferences handles GET /api/v1/organizations/{slug}/notifications/preferences.
func (h *Handler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	resp, err := h.svc.GetPreferences(r.Context(), slug, principal.UserID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// UpdatePreference handles PUT /api/v1/organizations/{slug}/notifications/preferences/{event_type}.
func (h *Handler) UpdatePreference(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	eventType := chi.URLParam(r, "event_type")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	var req UpdatePreferenceRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := h.svc.UpdatePreference(r.Context(), slug, eventType, principal.UserID, principal.UserID, req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// ── SSE stream endpoint ───────────────────────────────────────────────────────

// sseKeepaliveInterval controls how often a keepalive comment is sent to
// prevent proxy/load-balancer idle-connection timeouts.
const sseKeepaliveInterval = 25 * time.Second

// Stream handles GET /api/v1/organizations/{slug}/notifications/stream.
//
// Authentication: JWT via ?token=<jwt> query param (EventSource cannot set
// Authorization headers). Falls back to Authorization: Bearer <jwt> header
// for curl/testing convenience.
//
// The endpoint:
//  1. Validates the JWT and resolves the org from {slug}.
//  2. Ensures the JWT's org claim matches the requested org (tenant isolation).
//  3. Subscribes the connection to the in-process Hub.
//  4. Streams SSE frames until the client disconnects, the hub shuts down,
//     or the request context is cancelled.
//
// Each event frame:
//
//	event: notification\ndata: <JSON>\n\n
//
// Keepalive frames (every 25 s):
//
//	:\n\n
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	// Extract token from ?token= or Authorization: Bearer <token>.
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		tokenStr = bearerToken(r)
	}
	if tokenStr == "" {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	claims, err := auth.ValidateToken(tokenStr, h.cfg.JWTSecret)
	if err != nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	// Resolve org by slug and validate it matches the JWT's org claim.
	org, err := h.svc.repo.GetOrgBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		h.log.Error("notifications stream: resolve org", slog.Any("error", err))
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Tenant isolation: JWT org must match the requested org.
	// Platform-level tokens (OrganizationID == "") are rejected — SSE is org-scoped.
	if claims.OrganizationID != pgutil.UUIDToString(org.ID) {
		response.Error(w, http.StatusForbidden, "forbidden")
		return
	}

	// Require http.Flusher to push frames without buffering.
	flusher, ok := w.(http.Flusher)
	if !ok {
		response.Error(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	// Parse userID from JWT claims.
	userID, parseErr := pgutil.ParseUUID(claims.UserID)
	if parseErr != nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	// Subscribe — returns a buffered channel; Unsubscribe closes it on exit.
	ch := h.hub.Subscribe(org.ID, userID)
	defer h.hub.Unsubscribe(org.ID, userID, ch)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx proxy buffering

	// Send initial keepalive so the client knows the connection is live.
	fmt.Fprint(w, ":\n\n")
	flusher.Flush()

	ticker := time.NewTicker(sseKeepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case data, open := <-ch:
			if !open {
				// Channel closed by Hub.Shutdown — server is going away.
				return
			}
			fmt.Fprintf(w, "event: notification\ndata: %s\n\n", data)
			flusher.Flush()

		case <-ticker.C:
			fmt.Fprint(w, ":\n\n")
			flusher.Flush()

		case <-r.Context().Done():
			// Client disconnected.
			return

		case <-h.hub.Done():
			return
		}
	}
}

// bearerToken extracts the raw token from "Authorization: Bearer <token>".
// Returns "" if the header is absent or malformed.
func bearerToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(v) > len(prefix) && v[:len(prefix)] == prefix {
		return v[len(prefix):]
	}
	return ""
}

// ── error mapping ─────────────────────────────────────────────────────────────

func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusNotFound, "organization not found")
	case errors.Is(err, ErrNotificationNotFound):
		response.Error(w, http.StatusNotFound, "notification not found")
	case errors.Is(err, ErrPreferenceNotFound):
		response.Error(w, http.StatusNotFound, "preference not found")
	case errors.Is(err, ErrInvalidEventType):
		response.Error(w, http.StatusBadRequest, "invalid notification event type")
	case errors.Is(err, ErrInvalidChannel):
		response.Error(w, http.StatusBadRequest, "invalid notification channel")
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, "forbidden")
	default:
		h.log.Error("notifications: unhandled error",
			slog.String("path", r.URL.Path),
			slog.Any("error", err),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parsePagination(r *http.Request) (limit, offset int) {
	limit = 50
	offset = 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}
