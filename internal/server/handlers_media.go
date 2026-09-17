package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ffdkj/my_nav/internal/nav"
)

// ---------- 搜索引擎 ----------

func (h *handlers) createEngine(w http.ResponseWriter, r *http.Request) {
	var in nav.EngineInput
	if !decodeJSON(w, r, &in) {
		return
	}
	engine, err := h.svc.CreateEngine(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, engine)
}

func (h *handlers) updateEngine(w http.ResponseWriter, r *http.Request) {
	var in nav.EngineInput
	if !decodeJSON(w, r, &in) {
		return
	}
	engine, err := h.svc.UpdateEngine(r.Context(), chi.URLParam(r, "engineID"), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, engine)
}

func (h *handlers) deleteEngine(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteEngine(r.Context(), chi.URLParam(r, "engineID")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) reorderEngines(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := h.svc.ReorderEngines(r.Context(), in.IDs); err != nil {
		writeError(w, err)
		return
	}
	engines, err := h.svc.Bootstrap(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"engines": engines.Engines})
}

// ---------- 壁纸 ----------

const maxWallpaperUpload = 10 << 20 // 10 MiB，与 imageproc 的上限一致

func (h *handlers) listWallpapers(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListWallpapers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"wallpapers": items})
}

// createWallpaper 同时支持两种来源：
//   - multipart/form-data（file 字段）= 上传
//   - application/json（{id, remote_url}）= 登记外链
func (h *handlers) createWallpaper(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, maxWallpaperUpload)
		if err := r.ParseMultipartForm(maxWallpaperUpload); err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody("bad_upload", "expected a multipart form with a 'file' field"))
			return
		}
		id := r.FormValue("id")
		file, _, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody("bad_upload", "form field 'file' is required"))
			return
		}
		defer func() { _ = file.Close() }()
		data, err := io.ReadAll(io.LimitReader(file, maxWallpaperUpload+1))
		if err != nil || len(data) > maxWallpaperUpload {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorBody("too_large", "wallpaper must be at most 10 MiB"))
			return
		}
		item, err := h.svc.AddWallpaperUpload(r.Context(), id, data)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
		return
	}

	var in struct {
		ID        string `json:"id"`
		RemoteURL string `json:"remote_url"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("bad_json", "malformed JSON body"))
		return
	}
	item, err := h.svc.AddWallpaperURL(r.Context(), in.ID, in.RemoteURL)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *handlers) updateWallpaper(w http.ResponseWriter, r *http.Request) {
	var in nav.WallpaperUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	item, err := h.svc.UpdateWallpaper(r.Context(), chi.URLParam(r, "wallpaperID"), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *handlers) deleteWallpaper(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteWallpaper(r.Context(), chi.URLParam(r, "wallpaperID")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) materializeWallpaper(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.MaterializeWallpaper(r.Context(), chi.URLParam(r, "wallpaperID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// serveWallpaper 提供壁纸文件：/wallpapers/{orig|thumb}/<name>。
// 内容寻址 ⇒ 可以 immutable 缓存。
func (h *handlers) serveWallpaper(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	name := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	full, err := h.svc.WallpaperFile(kind, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if info, err := os.Stat(full); err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, full)
}
