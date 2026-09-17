package favicon

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// makePNG 生成一张 n×n 的纯色 PNG，用于模拟各种尺寸的图标。
func makePNG(t *testing.T, n int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for x := 0; x < n; x++ {
		for y := 0; y < n; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestFetchPrefersGoogle(t *testing.T) {
	icon := makePNG(t, 128)
	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("url") == "" {
			t.Errorf("google 端点必须带 url 参数")
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(icon)
	}))
	defer google.Close()

	f := NewFetcherWithEndpoints(google.URL, "http://127.0.0.1:1/ip3")
	res, err := f.Fetch(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.Source != "google" {
		t.Errorf("source = %s, want google", res.Source)
	}
	if res.Mime != "image/png" || res.Width != 128 {
		t.Errorf("got %s %dx%d, want image/png 128x128", res.Mime, res.Width, res.Height)
	}
}

func TestFetchFallsBackToDuckDuckGo(t *testing.T) {
	ico := []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x20, 0x20} // ICO 魔数 + 少量数据
	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound) // 无图标 → 404（哨兵地球 PNG 我们不关心）
	}))
	defer google.Close()
	ddg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".ico") {
			t.Errorf("ddg 路径 = %s, want *.ico", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/x-icon")
		_, _ = w.Write(ico)
	}))
	defer ddg.Close()

	f := NewFetcherWithEndpoints(google.URL, ddg.URL)
	res, err := f.Fetch(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.Source != "duckduckgo" || res.Mime != "image/x-icon" {
		t.Errorf("got source=%s mime=%s, want duckduckgo image/x-icon", res.Source, res.Mime)
	}
}

// 站点只给了 16x16 时，应该继续做自建发现去拿 apple-touch-icon 这种高清源。
func TestFetchUpgradesSmallGoogleIcon(t *testing.T) {
	small := makePNG(t, 16)
	big := makePNG(t, 180)

	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(small)
	}))
	defer google.Close()

	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head>
				<link rel="icon" type="image/png" sizes="16x16" href="/tiny.png">
				<link rel="apple-touch-icon" sizes="180x180" href="/big.png">
				</head></html>`))
		case "/big.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(big)
		case "/tiny.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(small)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer site.Close()

	f := NewFetcherWithEndpoints(google.URL, "http://127.0.0.1:1/ip3")
	res, err := f.Fetch(context.Background(), site.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.Source != "discovered" {
		t.Errorf("source = %s, want discovered", res.Source)
	}
	if res.Width != 180 {
		t.Errorf("width = %d, want 180 (apple-touch-icon 应优先于 16x16)", res.Width)
	}
}

func TestFetchDiscoversFaviconICO(t *testing.T) {
	ico := []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10}
	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer google.Close()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			w.Header().Set("Content-Type", "image/x-icon")
			_, _ = w.Write(ico)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>no icon links</title></head></html>`))
	}))
	defer site.Close()

	f := NewFetcherWithEndpoints(google.URL, "http://127.0.0.1:1/ip3")
	res, err := f.Fetch(context.Background(), site.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.Mime != "image/x-icon" {
		t.Errorf("mime = %s, want image/x-icon", res.Mime)
	}
}

func TestFetchReturnsNoIconWhenEverythingMisses(t *testing.T) {
	miss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer miss.Close()

	f := NewFetcherWithEndpoints(miss.URL, miss.URL)
	_, err := f.Fetch(context.Background(), miss.URL)
	if !errors.Is(err, ErrNoIcon) {
		t.Fatalf("err = %v, want ErrNoIcon", err)
	}
}

// 外网那两步不能把预算吃光，否则"自建发现"（本地/局域网站点唯一的出路）永远轮不到。
//
// 真实触发：DuckDuckGo 偶发 TLS 重置并一路挂到 5s，而总预算只有 3s，
// 于是首页明明声明了 apple-touch-icon 也被记成 miss（e2e/icons.mjs 因此变红）。
func TestFetchReservesBudgetForDiscovery(t *testing.T) {
	big := makePNG(t, 180)

	// 两个"外部服务"都挂住不响应，直到客户端放弃
	hang := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer hang.Close()

	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/big.png" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(big)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head>
			<link rel="apple-touch-icon" sizes="180x180" href="/big.png">
			</head></html>`))
	}))
	defer site.Close()

	f := NewFetcherWithEndpoints(hang.URL, hang.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	res, err := f.Fetch(ctx, site.URL)
	if err != nil {
		t.Fatalf("Fetch: %v（预算被外部两步吃光了？）", err)
	}
	if res.Source != "discovered" || res.Width != 180 {
		t.Errorf("got source=%s %dx%d, want discovered 180x180", res.Source, res.Width, res.Height)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("耗时 %v：单步限时没生效", elapsed)
	}
}

// 传送门必须关死：默认 fetcher 不许连私网/回环地址。
func TestFetchBlocksPrivateAddresses(t *testing.T) {
	icon := makePNG(t, 64)
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(icon)
	}))
	defer local.Close()

	// 注意：用真实默认构造，allowPrivate=false
	f := NewFetcher(nil, false)
	f.googleBase = local.URL
	f.ddgBase = local.URL

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := f.Fetch(ctx, local.URL); err == nil {
		t.Fatal("期望被 SSRF 防护拦下，结果却成功了")
	}
}

func TestSafeControlRejectsNonPublicIPs(t *testing.T) {
	blocked := []string{
		"127.0.0.1:80", "10.1.2.3:443", "192.168.1.1:80", "172.16.5.5:80",
		"169.254.169.254:80", "0.0.0.0:80", "[::1]:80", "224.0.0.1:80",
	}
	for _, addr := range blocked {
		if err := safeControl("tcp", addr, nil); err == nil {
			t.Errorf("safeControl(%q) = nil, 期望被拒绝", addr)
		}
	}
	if err := safeControl("tcp", "93.184.216.34:443", nil); err != nil {
		t.Errorf("safeControl(公网 IP) = %v, 期望通过", err)
	}
	if err := safeControl("tcp", "not-an-ip:443", nil); err == nil {
		t.Error("safeControl(无法解析的地址) 应为错误")
	}
}

func TestInspectRejectsJunkAndOversized(t *testing.T) {
	if _, _, _, err := Inspect([]byte("this is definitely not an image")); !errors.Is(err, ErrUnknownImage) {
		t.Errorf("junk err = %v, want ErrUnknownImage", err)
	}
	if _, _, _, err := Inspect(nil); !errors.Is(err, ErrUnknownImage) {
		t.Errorf("empty err = %v, want ErrUnknownImage", err)
	}
	huge := make([]byte, maxIconBytes+1)
	if _, _, _, err := Inspect(huge); err == nil {
		t.Error("超过字节上限应报错")
	}
	// SVG 与 ICO 不做解码，但要能识别
	if mime, _, _, err := Inspect([]byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`)); err != nil || mime != "image/svg+xml" {
		t.Errorf("svg: mime=%s err=%v", mime, err)
	}
	if mime, _, _, err := Inspect([]byte{0, 0, 1, 0, 2, 0}); err != nil || mime != "image/x-icon" {
		t.Errorf("ico: mime=%s err=%v", mime, err)
	}
}

func TestIconCandidatesPriority(t *testing.T) {
	body := []byte(`<html><head>
		<base href="https://cdn.example.com/assets/">
		<link rel="shortcut icon" href="legacy.ico">
		<link rel="icon" sizes="32x32" href="small.png">
		<link rel="icon" type="image/svg+xml" href="/logo.svg">
		<link rel="mask-icon" href="/mask.svg">
		<link rel="apple-touch-icon" sizes="180x180" href="touch.png">
	</head></html>`)

	got := iconCandidates(body, "https://example.com")
	if len(got) == 0 {
		t.Fatal("没解析出任何候选")
	}
	// SVG 优先级最高；<base> 要参与解析
	if got[0] != "https://cdn.example.com/logo.svg" {
		t.Errorf("首个候选 = %s, want https://cdn.example.com/logo.svg", got[0])
	}
	// base 生效：相对路径要基于 base 而不是 origin
	foundTouch := false
	for _, c := range got {
		if c == "https://cdn.example.com/assets/touch.png" {
			foundTouch = true
		}
	}
	if !foundTouch {
		t.Errorf("apple-touch-icon 未按 <base> 解析：%v", got)
	}
}

func TestNormalizeUploadScalesDown(t *testing.T) {
	out, mime, w, h, err := NormalizeUpload(makePNG(t, 512), 256)
	if err != nil {
		t.Fatalf("NormalizeUpload: %v", err)
	}
	if mime != "image/png" || w != 256 || h != 256 {
		t.Errorf("got %s %dx%d, want image/png 256x256", mime, w, h)
	}
	if len(out) == 0 {
		t.Error("输出为空")
	}
	// 小于上限的图不做缩放，原样返回
	src := makePNG(t, 64)
	out2, _, w2, _, err := NormalizeUpload(src, 256)
	if err != nil {
		t.Fatalf("NormalizeUpload(small): %v", err)
	}
	if w2 != 64 || !bytes.Equal(out2, src) {
		t.Errorf("小图不该被改动：w=%d", w2)
	}
}

// 代理支持：置上 HTTPS_PROXY 后，抓取必须真的走代理。
// 这是墙内主机能抓到图标的唯一途径，所以要有测试盯着，别被重构掉。
func TestFetcherHonoursProxyEnvironment(t *testing.T) {
	var proxied int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied++
		// 作为 HTTP 代理：请求行里带完整 URL，直接回一个 PNG 即可
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(makePNG(t, 64))
	}))
	defer proxy.Close()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")

	f := NewFetcher(nil, true) // allowPrivate：http 代理是回环地址
	// 目标用 http：走 HTTP 代理时 Go 会对 http 目标发"绝对 URI"的普通请求，
	// 而 https 目标要先 CONNECT 建隧道（假代理处理不了 TLS，测的就不是同一件事了）。
	f.googleBase = "http://favicon.invalid/faviconV2"
	res, err := f.Fetch(context.Background(), "http://site.invalid")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if proxied == 0 {
		t.Fatal("请求没有经过代理：ProxyFromEnvironment 没生效")
	}
	if res.Source != "google" {
		t.Errorf("source = %s, want google", res.Source)
	}
}
