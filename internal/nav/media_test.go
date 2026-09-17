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
	"testing"

	"github.com/ffdkj/my_nav/internal/db"
	"github.com/ffdkj/my_nav/internal/favicon"
	"github.com/ffdkj/my_nav/internal/migrate"
)

func newMediaService(t *testing.T) (*Service, context.Context, string) {
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

	orig := filepath.Join(dir, "wallpapers", "orig")
	thumb := filepath.Join(dir, "wallpapers", "thumb")
	svc := New(handle, WithMedia(
		favicon.NewStore(filepath.Join(dir, "icons")),
		favicon.NewStore(orig),
		favicon.NewStore(thumb),
		favicon.NewFetcher(nil, true),
		true,
	))
	return svc, ctx, dir
}

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestEngineCRUDAndBuiltinProtection(t *testing.T) {
	svc, ctx, _ := newMediaService(t)

	created, err := svc.CreateEngine(ctx, EngineInput{
		ID: "eng-mine", Name: "My Search", URLTpl: "https://example.com/s?q={query}",
		IconText: "M", IconColor: "#3B82F6",
	})
	if err != nil {
		t.Fatalf("CreateEngine: %v", err)
	}
	if created.IsBuiltin {
		t.Errorf("自定义引擎不该标记为内置")
	}

	updated, err := svc.UpdateEngine(ctx, "eng-mine", EngineInput{Name: "Renamed"})
	if err != nil {
		t.Fatalf("UpdateEngine: %v", err)
	}
	if updated.Name != "Renamed" {
		t.Errorf("name = %s, want Renamed（部分更新应保留其余字段）", updated.Name)
	}
	if updated.URLTpl != created.URLTpl {
		t.Errorf("未提供的字段被清空了：%s", updated.URLTpl)
	}

	// 模板缺 {query} 必须被拒（否则用户搜不出东西）
	if _, err := svc.CreateEngine(ctx, EngineInput{
		ID: "eng-bad", Name: "Bad", URLTpl: "https://example.com/s", IconText: "B", IconColor: "#EF4444",
	}); err == nil {
		t.Error("缺少 {query} 的模板应被拒绝")
	}

	// 内置引擎可以改名，但不能删
	if _, err := svc.UpdateEngine(ctx, "eng-google", EngineInput{Name: "Google CN"}); err != nil {
		t.Errorf("内置引擎应允许改名：%v", err)
	}
	err = svc.DeleteEngine(ctx, "eng-google")
	apiErr := codeOf(t, err)
	if apiErr.Code != "builtin_engine" {
		t.Errorf("code = %s, want builtin_engine", apiErr.Code)
	}

	if err := svc.DeleteEngine(ctx, "eng-mine"); err != nil {
		t.Fatalf("DeleteEngine(custom): %v", err)
	}
}

func TestDeleteDefaultEngineResetsDefault(t *testing.T) {
	svc, ctx, _ := newMediaService(t)

	if _, err := svc.CreateEngine(ctx, EngineInput{
		ID: "eng-tmp", Name: "Tmp", URLTpl: "https://t.dev/?q={query}", IconText: "T", IconColor: "#EF4444",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.UpdateSettings(ctx, map[string]string{"default_engine_id": "eng-tmp"}); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if err := svc.DeleteEngine(ctx, "eng-tmp"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	settings, err := svc.Settings(ctx)
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	if settings["default_engine_id"] != "eng-google" {
		t.Errorf("default_engine_id = %s, want eng-google（删掉默认引擎后要回落到内置）", settings["default_engine_id"])
	}
}

func TestWallpaperUploadGeneratesThumb(t *testing.T) {
	svc, ctx, dir := newMediaService(t)

	item, err := svc.AddWallpaperUpload(ctx, "wp1", testPNG(t, 2400, 1200))
	if err != nil {
		t.Fatalf("AddWallpaperUpload: %v", err)
	}
	if item.Kind != "upload" || item.File == nil || item.ThumbFile == nil {
		t.Fatalf("item = %+v, want an uploaded wallpaper with file+thumb", item)
	}
	if item.W == nil || *item.W != 1920 {
		t.Errorf("thumb width = %v, want 1920（超过 1920 宽要缩放）", item.W)
	}

	origPath := filepath.Join(dir, "wallpapers", "orig", *item.File)
	thumbPath := filepath.Join(dir, "wallpapers", "thumb", *item.ThumbFile)
	if _, err := os.Stat(origPath); err != nil {
		t.Errorf("原图未落盘：%v", err)
	}
	if _, err := os.Stat(thumbPath); err != nil {
		t.Errorf("缩略图未落盘：%v", err)
	}

	// 不支持的内容要拒绝
	if _, err := svc.AddWallpaperUpload(ctx, "wp-bad", []byte("not an image at all")); err == nil {
		t.Error("非图片内容应被拒绝")
	}
}

func TestWallpaperDeleteRemovesUnreferencedFiles(t *testing.T) {
	svc, ctx, dir := newMediaService(t)
	same := testPNG(t, 800, 600)

	first, err := svc.AddWallpaperUpload(ctx, "wp1", same)
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}
	second, err := svc.AddWallpaperUpload(ctx, "wp2", same)
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}
	// 同一张图内容寻址 → 同一路径，删除其中一个不能把文件删掉
	if *first.File != *second.File {
		t.Fatalf("内容寻址失效：%s vs %s", *first.File, *second.File)
	}
	path := filepath.Join(dir, "wallpapers", "orig", *first.File)

	if err := svc.DeleteWallpaper(ctx, "wp1"); err != nil {
		t.Fatalf("delete first: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("仍被 wp2 引用的文件不该被删除：%v", err)
	}

	if err := svc.DeleteWallpaper(ctx, "wp2"); err != nil {
		t.Fatalf("delete second: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("引用计数归零后文件应被删除，stat err = %v", err)
	}
}

func TestWallpaperMaterializeFromURL(t *testing.T) {
	svc, ctx, dir := newMediaService(t)

	image := testPNG(t, 1600, 900)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(image)
	}))
	defer server.Close()

	item, err := svc.AddWallpaperURL(ctx, "wp-url", server.URL+"/photo.png")
	if err != nil {
		t.Fatalf("AddWallpaperURL: %v", err)
	}
	if item.Kind != "url" || item.RemoteURL == nil {
		t.Fatalf("item = %+v, want a url wallpaper", item)
	}
	if item.File != nil {
		t.Error("仅登记外链时不该有本地文件")
	}

	materialized, err := svc.MaterializeWallpaper(ctx, "wp-url")
	if err != nil {
		t.Fatalf("MaterializeWallpaper: %v", err)
	}
	if materialized.Kind != "upload" || materialized.File == nil {
		t.Fatalf("materialize 后应变成本地文件：%+v", materialized)
	}
	if _, err := os.Stat(filepath.Join(dir, "wallpapers", "orig", *materialized.File)); err != nil {
		t.Errorf("下载的文件未落盘：%v", err)
	}

	// 目标不是图片时要给出可读的失败信息
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>not an image</html>"))
	}))
	defer bad.Close()
	if _, err := svc.AddWallpaperURL(ctx, "wp-bad", bad.URL+"/x"); err != nil {
		t.Fatalf("AddWallpaperURL(bad): %v", err)
	}
	if _, err := svc.MaterializeWallpaper(ctx, "wp-bad"); err == nil {
		t.Error("目标不是图片时应报错")
	}
}

func TestWallpaperFileResolverRejectsTraversal(t *testing.T) {
	svc, ctx, _ := newMediaService(t)
	if _, err := svc.AddWallpaperUpload(ctx, "wp1", testPNG(t, 100, 100)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	for _, bad := range []string{"../../etc/passwd", "/etc/passwd", ""} {
		if _, err := svc.WallpaperFile("orig", bad); err == nil {
			t.Errorf("WallpaperFile(orig, %q) 应被拒绝", bad)
		}
	}
	if _, err := svc.WallpaperFile("nope", "aa/x.jpg"); err == nil {
		t.Error("未知 kind 应被拒绝")
	}
}
