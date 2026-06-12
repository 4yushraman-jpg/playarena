package players

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/response"
	"github.com/4yushraman-jpg/playarena/internal/platform/validator"
)

// SelfHandler exposes the GP-1 global PlayerProfile endpoints over HTTP.
type SelfHandler struct {
	svc *SelfService
	log *slog.Logger
}

// NewSelfHandler constructs a SelfHandler.
func NewSelfHandler(svc *SelfService, log *slog.Logger) *SelfHandler {
	return &SelfHandler{svc: svc, log: log}
}

// immutableSelfFields are keys a client must not send to PATCH /me/player.
// They are not user-editable identity fields and are rejected explicitly so a
// caller cannot believe they changed them.
var immutableSelfFields = []string{"organization_id", "user_id", "status", "id"}

// CreateOwn handles POST /api/v1/me/player.
func (h *SelfHandler) CreateOwn(w http.ResponseWriter, r *http.Request) {
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	var req CreateProfileRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	profile, err := h.svc.CreateOwn(r.Context(), principal.UserID, req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	h.log.InfoContext(r.Context(), "players.me.create.success",
		slog.String("player_id", profile.ID),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusCreated, profile)
}

// GetOwn handles GET /api/v1/me/player.
func (h *SelfHandler) GetOwn(w http.ResponseWriter, r *http.Request) {
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}
	profile, err := h.svc.GetOwn(r.Context(), principal.UserID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, profile)
}

// UpdateOwn handles PATCH /api/v1/me/player.
func (h *SelfHandler) UpdateOwn(w http.ResponseWriter, r *http.Request) {
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	// Reject attempts to set immutable fields before decoding into the typed
	// struct (which would silently ignore them). The body is read once and then
	// handed back to DecodeJSON for normal validation.
	raw, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if field, found := containsImmutableField(raw); found {
		response.Error(w, http.StatusUnprocessableEntity, field+" cannot be modified via this endpoint")
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(raw))
	var req UpdateProfileRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	profile, err := h.svc.UpdateOwn(r.Context(), principal.UserID, req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, profile)
}

// GetByID handles GET /api/v1/players/{id} (visibility-aware global read).
func (h *SelfHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}
	id := chi.URLParam(r, "id")
	profile, err := h.svc.GetByIDGlobal(r.Context(), id, principal.UserID, principal.IsPlatformUser())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, profile)
}

// ── helpers ────────────────────────────────────────────────────────────────────

func containsImmutableField(raw []byte) (string, bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", false
	}
	for _, f := range immutableSelfFields {
		if _, ok := m[f]; ok {
			return f, true
		}
	}
	return "", false
}

func (h *SelfHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrProfileExists):
		response.Error(w, http.StatusConflict, "a player profile already exists for this user")
	case errors.Is(err, ErrPlayerNotFound):
		response.Error(w, http.StatusNotFound, "player profile not found")
	case errors.Is(err, ErrInvalidVisibility),
		errors.Is(err, ErrInvalidDominantHand),
		errors.Is(err, ErrInvalidNationality),
		errors.Is(err, ErrInvalidDateOfBirth):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		h.log.ErrorContext(r.Context(), "players.me.unexpected_error",
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

func (h *SelfHandler) writeDecodeError(w http.ResponseWriter, err error) {
	var ve *validator.ValidationError
	if errors.As(err, &ve) {
		response.Write(w, http.StatusBadRequest, struct {
			Error  string            `json:"error"`
			Fields map[string]string `json:"fields"`
		}{
			Error:  "validation failed",
			Fields: ve.Fields,
		})
		return
	}
	if strings.Contains(err.Error(), "too large") {
		response.Error(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	response.Error(w, http.StatusBadRequest, err.Error())
}
