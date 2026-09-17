package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/ffdkj/my_nav/internal/nav"
)

const maxImportBytes = 64 << 20 // 64 MiB：含壁纸的 zip 可能不小

// export 导出：默认给人读的 JSON，?withAssets=1 给含二进制的 zip。
func (h *handlers) export(w http.ResponseWriter, r *http.Request) {
	stamp := time.Now().UTC().Format("20060102-150405")

	if r.URL.Query().Get("withAssets") != "" {
		data, err := h.svc.ExportZip(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=%q", "my_nav-"+stamp+".zip"))
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		_, _ = w.Write(data)
		return
	}

	data, err := h.svc.ExportJSON(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", "my_nav-"+stamp+".json"))
	_, _ = w.Write(data)
}

// importData 全量覆盖导入（multipart，字段名 file）。
// 导入前会在服务器上留一份唯一的 pre-import 快照。
func (h *handlers) importData(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("bad_upload", "expected a multipart form with a 'file' field"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("bad_upload", "form field 'file' is required"))
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	if err != nil || len(data) > maxImportBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorBody("too_large", "import file must be at most 64 MiB"))
		return
	}

	doc, assets, err := nav.ParseImport(header.Filename, data)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.Import(r.Context(), doc, assets); err != nil {
		writeError(w, err)
		return
	}

	boot, err := h.svc.Bootstrap(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, boot)
}

func (h *handlers) backupInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.BackupInfo())
}

// backupDownload 提供导入前快照的下载（否则这份备份用户看不见也拿不走）。
func (h *handlers) backupDownload(w http.ResponseWriter, r *http.Request) {
	info := h.svc.BackupInfo()
	if !info.Exists {
		writeError(w, nav.NotFound("backup"))
		return
	}
	path := h.svc.BackupPath()
	if _, err := os.Stat(path); err != nil {
		writeError(w, nav.NotFound("backup"))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pre-import.json"`)
	http.ServeFile(w, r, path)
}
