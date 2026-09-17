package nav

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ffdkj/my_nav/internal/db"
	"github.com/ffdkj/my_nav/internal/favicon"
	"github.com/ffdkj/my_nav/internal/migrate"
)

// iconSite 起一个只声明 apple-touch-icon 的站点，并统计首页被请求了几次
// （用来验证"同一 URL 60 秒内不重复抓"）。
func iconSite(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	icon := alphaPNG(t, 180)
	var hits int32

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><link rel="apple-touch-icon" sizes="180x180" href="/touch.png"></head></html>`))
	})
	mux.HandleFunc("/touch.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(icon)
	})
	site := httptest.NewServer(mux)
	t.Cleanup(site.Close)
	return site, &hits
}

func alphaPNG(t *testing.T, n int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for x := 0; x < n; x++ {
		for y := 0; y < n; y++ {
			a := uint8(255)
			if x < n/3 && y < n/3 {
				a = 0
			}
			img.Set(x, y, color.RGBA{R: 30, G: 90, B: 200, A: a})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// newIconService 建一个带真实 Fetcher（指向本地站点）与图标仓库的服务。
func newIconService(t *testing.T, googleBase, ddgBase string) (*Service, context.Context, string) {
	t.Helper()
	dir := t.TempDir()
	handle, err := db.Open(filepath.Join(dir, "nav.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	ctx := context.Background()
	if _, err := migrate.Run(ctx, handle, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := New(handle, WithMedia(
		favicon.NewStore(filepath.Join(dir, "icons")),
		favicon.NewStore(filepath.Join(dir, "wallpapers", "orig")),
		favicon.NewStore(filepath.Join(dir, "wallpapers", "thumb")),
		favicon.NewFetcherWithEndpoints(googleBase, ddgBase),
		true,
	))
	return svc, ctx, dir
}

func TestIconCandidatesStoresBytesAndPickApplies(t *testing.T) {
	site, hits := iconSite(t)
	miss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer miss.Close()

	svc, ctx, dir := newIconService(t, miss.URL, miss.URL)

	// 先有一个链接（PutBoard 会顺手抓一次标准 favicon：本地站点没有 /favicon.ico → miss）
	board, err := svc.PutBoard(ctx, testPage, &BoardPayload{
		Revision: 0,
		Items:    []ItemDTO{linkItem("it-1", "l-1", 0, 0)},
		NewLinks: []LinkInput{{ID: "l-1", URL: site.URL, Title: "Local"}},
	})
	if err != nil {
		t.Fatalf("PutBoard: %v", err)
	}
	if len(board.Links) != 1 {
		t.Fatalf("board links = %d, want 1", len(board.Links))
	}

	// ⚠️ PutBoard 自己也会抓一次图标（自动链路第 3 步会 GET 首页），
	// 所以计数要在这里取基准，不能从 0 算。
	before := atomic.LoadInt32(hits)

	res, err := svc.IconCandidates(ctx, site.URL)
	if err != nil {
		t.Fatalf("IconCandidates: %v", err)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("候选数 = %d, want 1: %+v", len(res.Candidates), res.Candidates)
	}
	c := res.Candidates[0]
	if c.Source != "apple-touch-icon" || c.Width != 180 || c.Mime != "image/png" {
		t.Errorf("候选 = %+v, want apple-touch-icon 180 image/png", c)
	}
	if c.Alpha == nil || !*c.Alpha {
		t.Error("这张图是透明底，alpha 应为 true")
	}
	// 字节必须已经落盘（前端直接拿 icon_path 当缩略图，选中时不再下载）
	if _, err := os.Stat(filepath.Join(dir, "icons", filepath.FromSlash(c.IconPath))); err != nil {
		t.Errorf("候选字节没有落盘：%v", err)
	}

	// 同一 URL 再查一次：走 60s 缓存，不该再打对方站点
	if _, err := svc.IconCandidates(ctx, site.URL); err != nil {
		t.Fatalf("IconCandidates(cached): %v", err)
	}
	if got := atomic.LoadInt32(hits); got != before+1 {
		t.Errorf("首页命中 %d 次，want %d（第二次查询应命中 60s 缓存）", got, before+1)
	}

	// 采用这张候选
	picked, err := svc.PickIcon(ctx, "l-1", PickIconInput{IconPath: c.IconPath, RemoteURL: c.Remote})
	if err != nil {
		t.Fatalf("PickIcon: %v", err)
	}
	if picked.IconPath == nil || *picked.IconPath != c.IconPath {
		t.Errorf("icon_path = %v, want %s", picked.IconPath, c.IconPath)
	}
	if picked.IconStatus != "ok" || picked.IconSource != "auto" {
		t.Errorf("status/source = %s/%s, want ok/auto", picked.IconStatus, picked.IconSource)
	}
	if picked.IconW == nil || *picked.IconW != 180 || picked.IconH == nil || *picked.IconH != 180 {
		t.Errorf("尺寸 = %v x %v, want 180x180（服务端要重新 Inspect，不信前端）", picked.IconW, picked.IconH)
	}
	if picked.IconPickedURL == nil || *picked.IconPickedURL != c.Remote {
		t.Errorf("icon_picked_url = %v, want %s", picked.IconPickedURL, c.Remote)
	}

	// 改 URL：手选的图标不能被抹掉（spec §6 的守卫）
	moved, err := svc.UpdateLinkFields(ctx, "l-1", UpdateLinkInput{URL: ptr("https://moved.example.com/")})
	if err != nil {
		t.Fatalf("UpdateLinkFields: %v", err)
	}
	if moved.IconPath == nil || *moved.IconPath != c.IconPath {
		t.Errorf("改网址后手选图标丢了：icon_path = %v", moved.IconPath)
	}
	if moved.IconPickedURL == nil {
		t.Error("改网址后 icon_picked_url 被清空了")
	}

	// 主动重新抓取 = 覆盖手选（本地站点没有 favicon.ico → miss）
	refetched, err := svc.FetchAndStoreIcon(ctx, "l-1")
	if err != nil {
		t.Fatalf("FetchAndStoreIcon: %v", err)
	}
	if refetched.IconPickedURL != nil {
		t.Errorf("重新抓取后 icon_picked_url 应为空，got %v", *refetched.IconPickedURL)
	}
	if refetched.IconStatus != "miss" {
		t.Errorf("本地站点没有图标，状态应为 miss，got %s", refetched.IconStatus)
	}
}

func TestPickIconRejectsUnsafePath(t *testing.T) {
	svc, ctx, _ := newMediaService(t)
	if _, err := svc.PutBoard(ctx, testPage, &BoardPayload{
		Revision: 0,
		Items:    []ItemDTO{linkItem("it-1", "l-1", 0, 0)},
		NewLinks: []LinkInput{{ID: "l-1", URL: "https://example.com/", Title: "X"}},
	}); err != nil {
		t.Fatalf("PutBoard: %v", err)
	}

	for _, bad := range []string{"", "../../../etc/passwd", "/etc/passwd", "no-such-file.png"} {
		_, err := svc.PickIcon(ctx, "l-1", PickIconInput{IconPath: bad})
		if err == nil {
			t.Fatalf("PickIcon(%q) 应当报错", bad)
		}
		if code := codeOf(t, err).Code; code != "bad_request" {
			t.Errorf("PickIcon(%q) code = %s, want bad_request", bad, code)
		}
	}

	// 链接不存在 → 404 语义
	if _, err := svc.PickIcon(ctx, "nope", PickIconInput{IconPath: "aa/bb.png"}); codeOf(t, err).Code != "not_found" {
		t.Errorf("不存在的链接应当报 not_found")
	}
}

func TestTileShapeSettingValidation(t *testing.T) {
	svc, ctx, _ := newMediaService(t)

	settings, err := svc.Settings(ctx)
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	// 迁移 002 会给新库写上默认值
	if settings["tile_shape"] != "rounded" {
		t.Errorf("tile_shape 默认值 = %q, want rounded", settings["tile_shape"])
	}

	if err := svc.UpdateSettings(ctx, map[string]string{"tile_shape": "circle"}); err != nil {
		t.Fatalf("UpdateSettings(circle): %v", err)
	}
	settings, _ = svc.Settings(ctx)
	if settings["tile_shape"] != "circle" {
		t.Errorf("tile_shape = %q, want circle", settings["tile_shape"])
	}

	err = svc.UpdateSettings(ctx, map[string]string{"tile_shape": "hexagon"})
	if code := codeOf(t, err).Code; code != "bad_request" {
		t.Errorf("非法形状 code = %s, want bad_request", code)
	}
	if err := svc.UpdateSettings(ctx, map[string]string{"not_a_setting": "1"}); codeOf(t, err).Code != "bad_request" {
		t.Error("白名单外的键应被拒绝")
	}
}

func ptr[T any](v T) *T { return &v }
