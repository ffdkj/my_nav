// Package web 把前端构建产物内嵌进二进制。
//
// 构建流程：`npm --prefix web run build` 输出到 internal/web/dist，
// 随后 `go build` 通过 go:embed 打进去 —— 因此生产只有一个二进制。
//
// 注意：//go:embed 在目录不存在或为空时 **编译失败**，所以 dist/.gitkeep 必须入库；
// 未构建前端时用 placeholder 页面兜底，保证 `go build` 与 `go vet` 在新克隆里也能过。
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

const placeholder = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>my_nav · 前端未构建</title>
<style>body{background:#070b14;color:#e5edff;font:16px/1.6 system-ui;margin:0;
display:grid;place-items:center;height:100vh}code{background:#16203a;padding:.2em .5em;border-radius:.4em}</style>
</head><body><main>
<h1>前端尚未构建</h1>
<p>运行 <code>make web-build</code>（或 <code>npm --prefix web run build</code>）后重新编译 Go 二进制。</p>
<p>开发模式请用 <code>make dev</code> 起 Vite（:5180）并访问该端口。</p>
</main></body></html>
`

// Handler 返回 SPA 处理器：
//   - 命中磁盘文件则按类型设置缓存头（/assets/* 不可变，其余 no-cache）
//   - 未命中则回退到 index.html（客户端路由），未构建时回退到 placeholder
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("web: embed dist 目录缺失: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))
	indexHTML, indexErr := fs.ReadFile(sub, "index.html")
	hasIndex := indexErr == nil

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}

		if f, err := sub.Open(name); err == nil {
			_ = f.Close()
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if hasIndex {
			_, _ = w.Write(indexHTML)
			return
		}
		_, _ = w.Write([]byte(placeholder))
	})
}
