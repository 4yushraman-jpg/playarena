package public

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/4yushraman-jpg/playarena/internal/platform/response"
)

// Handler serves the anonymous public read API. Every endpoint is GET-only and
// maps the single ErrNotFound to 404 — there is no 403 and no existence leak.
type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// cacheable sets a short shared-cache TTL so public pages are CDN/proxy-cacheable
// and scraping pressure is absorbed. Live views change as matches conclude.
func cacheable(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "public, max-age=20")
}

func (h *Handler) Tournament(w http.ResponseWriter, r *http.Request) {
	orgSlug := chi.URLParam(r, "orgSlug")
	tSlug := chi.URLParam(r, "tournamentSlug")
	out, err := h.svc.Tournament(r.Context(), orgSlug, tSlug)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	cacheable(w)
	response.Write(w, http.StatusOK, out)
}

func (h *Handler) Matches(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Matches(r.Context(), chi.URLParam(r, "orgSlug"), chi.URLParam(r, "tournamentSlug"))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	cacheable(w)
	response.Write(w, http.StatusOK, out)
}

func (h *Handler) Standings(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Standings(r.Context(), chi.URLParam(r, "orgSlug"), chi.URLParam(r, "tournamentSlug"))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	cacheable(w)
	response.Write(w, http.StatusOK, out)
}

func (h *Handler) Match(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Match(r.Context(),
		chi.URLParam(r, "orgSlug"), chi.URLParam(r, "tournamentSlug"), chi.URLParam(r, "matchId"))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	cacheable(w)
	response.Write(w, http.StatusOK, out)
}

// writeErr maps ErrNotFound → 404; anything else → 500 (logged). Never 403.
func (h *Handler) writeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	h.log.Error("public.read.failed", slog.Any("error", err))
	response.Error(w, http.StatusInternalServerError, "internal server error")
}
