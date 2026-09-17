package server

import (
	"io"
	"net/http"
	"os"
	"strings"

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

// iconCandidates 处理 POST /api/icons/candidates：给一个网址，返回几张可用图标。
// 不需要 link 行 —— 新增链接时（link 还不存在）也要能先让用户挑。
func (h *handlers) iconCandidates(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL string `json:"url"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.IconCandidates(r.Context(), in.URL)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// pickIcon 处理 POST /api/links/{linkID}/icon/pick：采用用户选中的那张候选图标。
func (h *handlers) pickIcon(w http.ResponseWriter, r *http.Request) {
	var in nav.PickIconInput
	if !decodeJSON(w, r, &in) {
		return
	}
	link, err := h.svc.PickIcon(r.Context(), chi.URLParam(r, "linkID"), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
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

func (h *handlers) moveItem(w http.ResponseWriter, r *http.Request) {
	var in nav.MoveItemInput
	if !decodeJSON(w, r, &in) {
		return
	}
	board, err := h.svc.MoveItem(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, board)
}

// ---------- 图标 ----------

const maxUploadBytes = 1 << 20 // 1 MiB（前端限制 512KB，这里再兜一层）

func (h *handlers) refetchIcon(w http.ResponseWriter, r *http.Request) {
	link, err := h.svc.FetchAndStoreIcon(r.Context(), chi.URLParam(r, "linkID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (h *handlers) resetIcon(w http.ResponseWriter, r *http.Request) {
	link, err := h.svc.ResetIcon(r.Context(), chi.URLParam(r, "linkID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (h *handlers) setMonogram(w http.ResponseWriter, r *http.Request) {
	var in nav.MonogramInput
	if !decodeJSON(w, r, &in) {
		return
	}
	link, err := h.svc.SetMonogram(r.Context(), chi.URLParam(r, "linkID"), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (h *handlers) uploadIcon(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("bad_upload", "expected a multipart form with a 'file' field"))
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("bad_upload", "form field 'file' is required"))
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil || len(data) > maxUploadBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorBody("too_large", "icon must be at most 1 MiB"))
		return
	}

	link, err := h.svc.StoreUploadedIcon(r.Context(), chi.URLParam(r, "linkID"), data)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

// serveIcon 提供图标文件。路径是内容寻址的，所以可以放心 immutable 缓存。
func (h *handlers) serveIcon(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	full, err := h.svc.IconFile(rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.HasSuffix(full, ".svg") {
		// SVG 只当 <img> 用；即便被直接打开也不给任何执行能力
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		w.Header().Set("Content-Type", "image/svg+xml")
	}
	http.ServeFile(w, r, full)
}
