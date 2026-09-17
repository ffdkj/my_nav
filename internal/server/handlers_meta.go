package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ffdkj/my_nav/internal/nav"
)

func (h *handlers) listLinks(w http.ResponseWriter, r *http.Request) {
	links, err := h.svc.ListAllLinks(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": links})
}

func (h *handlers) updateLink(w http.ResponseWriter, r *http.Request) {
	var in nav.UpdateLinkInput
	if !decodeJSON(w, r, &in) {
		return
	}
	link, err := h.svc.UpdateLinkFields(r.Context(), chi.URLParam(r, "linkID"), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (h *handlers) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.svc.Settings(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *handlers) patchSettings(w http.ResponseWriter, r *http.Request) {
	var updates map[string]string
	if !decodeJSON(w, r, &updates) {
		return
	}
	if err := h.svc.UpdateSettings(r.Context(), updates); err != nil {
		writeError(w, err)
		return
	}
	settings, err := h.svc.Settings(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}
