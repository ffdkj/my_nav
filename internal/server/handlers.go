package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ffdkj/my_nav/internal/nav"
)

const maxBodyBytes = 1 << 20 // 整板负载上限 1 MiB，足够几百个图标

type handlers struct {
	svc *nav.Service
}

// ---------- bootstrap / pages ----------

func (h *handlers) bootstrap(w http.ResponseWriter, r *http.Request) {
	data, err := h.svc.Bootstrap(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (h *handlers) listPages(w http.ResponseWriter, r *http.Request) {
	pages, err := h.svc.ListPages(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": pages})
}

func (h *handlers) createPage(w http.ResponseWriter, r *http.Request) {
	var in nav.CreatePageInput
	if !decodeJSON(w, r, &in) {
		return
	}
	page, err := h.svc.CreatePage(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, page)
}

func (h *handlers) updatePage(w http.ResponseWriter, r *http.Request) {
	var in nav.UpdatePageInput
	if !decodeJSON(w, r, &in) {
		return
	}
	page, err := h.svc.UpdatePage(r.Context(), chi.URLParam(r, "pageID"), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *handlers) deletePage(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeletePage(r.Context(), chi.URLParam(r, "pageID")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- board ----------

func (h *handlers) getBoard(w http.ResponseWriter, r *http.Request) {
	board, err := h.svc.GetBoard(r.Context(), chi.URLParam(r, "pageID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}

func (h *handlers) putBoard(w http.ResponseWriter, r *http.Request) {
	var payload nav.BoardPayload
	if !decodeJSON(w, r, &payload) {
		return
	}
	board, err := h.svc.PutBoard(r.Context(), chi.URLParam(r, "pageID"), &payload)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}

// resolvePageSlug 允许前端用 slug 或 id 访问（URL 里是 #/p/<slug>）。
func (h *handlers) resolvePageSlug(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(chi.URLParam(r, "slug"))
	if slug == "" {
		writeError(w, nav.BadRequest("slug is required"))
		return
	}
	pages, err := h.svc.ListPages(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	for _, p := range pages {
		if p.Slug == slug || p.ID == slug {
			writeJSON(w, http.StatusOK, p)
			return
		}
	}
	writeError(w, nav.NotFound("page"))
}

// ---------- helpers ----------

// decodeJSON 解析请求体；失败时已经写过响应，返回 false。
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorBody("payload_too_large", "request body exceeds 1 MiB"))
			return false
		}
		writeJSON(w, http.StatusBadRequest, errorBody("bad_json", "malformed JSON body: "+err.Error()))
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, err error) {
	apiErr := nav.AsError(err)
	if apiErr == nil {
		apiErr = &nav.Error{Status: http.StatusInternalServerError, Code: "internal", Message: "unknown error"}
	}
	payload := map[string]any{"code": apiErr.Code, "message": apiErr.Message}
	if len(apiErr.Conflicts) > 0 {
		payload["conflicts"] = apiErr.Conflicts
	}

	if apiErr.Status >= http.StatusInternalServerError {
		// 内部错误只记日志，不回显细节
		slog.Error("request failed", "code", apiErr.Code, "err", err)
		payload["message"] = "internal error"
	}
	writeJSON(w, apiErr.Status, map[string]any{"error": payload})
}

func errorBody(code, message string) map[string]any {
	return map[string]any{"error": map[string]string{"code": code, "message": message}}
}
