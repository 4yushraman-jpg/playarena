package match_events

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/response"
	"github.com/4yushraman-jpg/playarena/internal/platform/validator"
)

// Handler exposes the match events service over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ── endpoints ─────────────────────────────────────────────────────────────────

// Create handles POST /api/v1/organizations/{slug}/matches/{matchId}/events.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	matchID := chi.URLParam(r, "matchId")

	var req CreateRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	event, err := h.svc.Create(r.Context(), slug, matchID, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "match_events.create.failed",
			slog.String("org_slug", slug),
			slog.String("match_id", matchID),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeEventError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "match_events.create.success",
		slog.String("event_id", event.ID),
		slog.String("match_id", matchID),
		slog.Int64("sequence_number", event.SequenceNumber),
		slog.String("event_type", event.EventType),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusCreated, event)
}

// List handles GET /api/v1/organizations/{slug}/matches/{matchId}/events.
// Supports ?limit=, ?offset=, ?effective_only= query parameters.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	matchID := chi.URLParam(r, "matchId")

	params := ListParams{
		Limit:  DefaultListLimit,
		Offset: 0,
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			params.Limit = int32(n)
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			params.Offset = int32(n)
		}
	}
	if r.URL.Query().Get("effective_only") == "true" {
		params.EffectiveOnly = true
	}

	list, err := h.svc.List(r.Context(), slug, matchID, params)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		if errors.Is(err, ErrMatchNotFound) {
			response.Error(w, http.StatusNotFound, "match not found")
			return
		}
		h.log.ErrorContext(r.Context(), "match_events.list.failed",
			slog.String("org_slug", slug),
			slog.String("match_id", matchID),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, list)
}

// GetByID handles GET /api/v1/organizations/{slug}/matches/{matchId}/events/{eventId}.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	matchID := chi.URLParam(r, "matchId")
	eventID := chi.URLParam(r, "eventId")

	event, err := h.svc.GetByID(r.Context(), slug, matchID, eventID)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		if errors.Is(err, ErrMatchNotFound) {
			response.Error(w, http.StatusNotFound, "match not found")
			return
		}
		if errors.Is(err, ErrEventNotFound) {
			response.Error(w, http.StatusNotFound, "match event not found")
			return
		}
		h.log.ErrorContext(r.Context(), "match_events.get.failed",
			slog.String("org_slug", slug),
			slog.String("match_id", matchID),
			slog.String("event_id", eventID),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, event)
}

// ── error handling ────────────────────────────────────────────────────────────

func (h *Handler) writeEventError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusNotFound, "organization not found")
	case errors.Is(err, ErrMatchNotFound):
		response.Error(w, http.StatusNotFound, "match not found")
	case errors.Is(err, ErrEventNotFound):
		response.Error(w, http.StatusNotFound, "match event not found")
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrInvalidEventType), errors.Is(err, ErrInvalidTimestamp),
		errors.Is(err, ErrInvalidPayload), errors.Is(err, ErrInvalidScorePayload):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrDuplicateLifecycleEvent):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrMatchNotLive),
		errors.Is(err, ErrTeamNotParticipant),
		errors.Is(err, ErrPlayerNotParticipant),
		errors.Is(err, ErrPlayerNotOnTeam),
		errors.Is(err, ErrCancelsEventRequired),
		errors.Is(err, ErrCancelsEventNotAllowed),
		errors.Is(err, ErrCancelsEventNotFound),
		errors.Is(err, ErrCancelsEventCrossMatch),
		errors.Is(err, ErrEventAlreadyCancelled),
		errors.Is(err, ErrCannotCancelCorrection):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		h.log.ErrorContext(r.Context(), "match_events.unexpected_error",
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

func (h *Handler) writeDecodeError(w http.ResponseWriter, err error) {
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
	response.Error(w, http.StatusBadRequest, err.Error())
}

func errKind(err error) string {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		return "org_not_found"
	case errors.Is(err, ErrMatchNotFound):
		return "match_not_found"
	case errors.Is(err, ErrEventNotFound):
		return "event_not_found"
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrMatchNotLive):
		return "match_not_live"
	case errors.Is(err, ErrInvalidEventType):
		return "invalid_event_type"
	case errors.Is(err, ErrInvalidPayload):
		return "invalid_payload"
	case errors.Is(err, ErrInvalidScorePayload):
		return "invalid_score_payload"
	case errors.Is(err, ErrInvalidTimestamp):
		return "invalid_timestamp"
	case errors.Is(err, ErrTeamNotParticipant):
		return "team_not_participant"
	case errors.Is(err, ErrPlayerNotParticipant):
		return "player_not_participant"
	case errors.Is(err, ErrPlayerNotOnTeam):
		return "player_not_on_team"
	case errors.Is(err, ErrDuplicateLifecycleEvent):
		return "duplicate_lifecycle_event"
	case errors.Is(err, ErrCancelsEventRequired):
		return "cancels_event_required"
	case errors.Is(err, ErrCancelsEventNotAllowed):
		return "cancels_event_not_allowed"
	case errors.Is(err, ErrCancelsEventNotFound):
		return "cancels_event_not_found"
	case errors.Is(err, ErrCancelsEventCrossMatch):
		return "cancels_event_cross_match"
	case errors.Is(err, ErrEventAlreadyCancelled):
		return "event_already_cancelled"
	case errors.Is(err, ErrCannotCancelCorrection):
		return "cannot_cancel_correction"
	default:
		return "internal_error"
	}
}
