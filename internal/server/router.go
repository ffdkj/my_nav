// Package server 装配 HTTP 路由。
//
// 三条硬约束（来自 docs/research/02-backend-go-sqlc-docker.md，已实测）：
//  1. 禁用 middleware.RealIP —— 它被三条安全公告标记为可伪造来源 IP（会改写 r.RemoteAddr）。
//     改用 ClientIPFromRemoteAddr；将来若加反向代理再用 ClientIPFromXFFTrustedProxies(n)。
//  2. 未知 /api/* 必须在 /api 分组内返回 JSON 404；只挂根 NotFound 会让 /api/nope 返回 SPA 的 200 HTML。
//  3. chi 的 NotFound/Get 需要 http.HandlerFunc，不接受 http.Handler。
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ffdkj/my_nav/internal/config"
	"github.com/ffdkj/my_nav/internal/web"
)

type server struct {
	cfg config.Config
}

// New 返回装配好的根 handler。
func New(cfg config.Config) http.Handler {
	s := &server{cfg: cfg}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.ClientIPFromRemoteAddr)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(20 * time.Second))
	r.Use(accessLog)

	r.Get("/healthz", s.health)

	r.Route("/api", func(api chi.Router) {
		api.Get("/bootstrap", s.bootstrap)
		// 保持最后注册：任何未匹配的 /api 路径都返回 JSON 404。
		api.NotFound(jsonNotFound)
		api.HandleFunc("/*", jsonNotFound)
	})

	// SPA 回退：非 /api 路径交给内嵌前端。
	spa := web.Handler()
	r.NotFound(spa.ServeHTTP)
	r.Get("/*", spa.ServeHTTP)
	r.Get("/", spa.ServeHTTP)

	return r
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// bootstrap 在 M0 只是一个可用的空壳（M1 接入 sqlc 查询后填充真实数据）。
func (s *server) bootstrap(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"pages":    []any{},
		"settings": map[string]string{},
		"engines":  []any{},
		"revision": 0,
	})
}

func jsonNotFound(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]any{
		"error": map[string]string{"code": "not_found", "message": "no such API route"},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Debug("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}
