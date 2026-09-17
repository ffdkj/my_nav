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
)

// makeAlphaPNG 生成一张 n×n 的 PNG，左上角四分之一是透明的（用来测 HasAlpha）。
func makeAlphaPNG(t *testing.T, n int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for x := 0; x < n; x++ {
		for y := 0; y < n; y++ {
			a := uint8(255)
			if x < n/2 && y < n/2 {
				a = 0
			}
			img.Set(x, y, color.RGBA{R: 20, G: 120, B: 220, A: a})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// makeICO 造一个 ICO 头（不解码像素的解析器只看头），entries 是各子图边长。
func makeICO(entries ...int) []byte {
	out := []byte{0x00, 0x00, 0x01, 0x00, byte(len(entries)), 0x00}
	for _, n := range entries {
		b := byte(n)
		if n == 256 {
			b = 0 // ICO 用 0 表示 256
		}
		entry := make([]byte, 16)
		entry[0], entry[1] = b, b
		out = append(out, entry...)
	}
	return append(out, 0x00) // 一点"像素数据"，解析器不需要
}

func TestICOSizePicksLargestEntry(t *testing.T) {
	w, h, ok := ICOSize(makeICO(16, 32, 48))
	if !ok || w != 48 || h != 48 {
		t.Errorf("ICOSize = %d,%d,%v; want 48,48,true", w, h, ok)
	}
	// 0 表示 256
	w, h, ok = ICOSize(makeICO(16, 256))
	if !ok || w != 256 {
		t.Errorf("ICOSize(256) = %d,%d,%v; want 256,256,true", w, h, ok)
	}
	if _, _, ok := ICOSize([]byte{0x00, 0x00, 0x01, 0x00}); ok {
		t.Error("截断的 ICO 头不该被接受")
	}
	if _, _, ok := ICOSize(makePNG(t, 8)); ok {
		t.Error("PNG 不该被当成 ICO")
	}
}

func TestInspectReportsICOSize(t *testing.T) {
	// 之前 ICO 一律返回 0,0，导致候选既排不了序也过不了"<64px 丢掉"的规则
	_, w, h, err := Inspect(makeICO(16, 64))
	if err != nil {
		t.Fatalf("Inspect(ico): %v", err)
	}
	if w != 64 || h != 64 {
		t.Errorf("ico 尺寸 = %dx%d, want 64x64", w, h)
	}
}

func TestHasAlphaDetectsTransparency(t *testing.T) {
	if got := HasAlpha(makeAlphaPNG(t, 64)); got == nil || !*got {
		t.Errorf("带透明像素的 PNG 应当判为透明，got %v", got)
	}
	if got := HasAlpha(makePNG(t, 64)); got == nil || *got {
		t.Errorf("全不透明的 PNG 应当判为不透明，got %v", got)
	}
	if got := HasAlpha([]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)); got != nil {
		t.Errorf("SVG 判不了，应当是 nil，got %v", *got)
	}
	if got := HasAlpha(makeICO(32)); got != nil {
		t.Errorf("ICO 判不了，应当是 nil，got %v", *got)
	}
	// 1×1 不透明 JPEG
	jpg := []byte{
		0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00, 0x03, 0x02, 0x02, 0x03, 0x02, 0x02, 0x03,
	}
	if got := HasAlpha(jpg); got != nil && *got {
		t.Error("解不开的字节流不该被判为透明")
	}
}

func TestCandidateScorePrefersVectorAndLargeTransparent(t *testing.T) {
	yes, no := true, false
	vector := Candidate{Mime: "image/svg+xml", Source: "link-icon"}
	bigAlpha := Candidate{Mime: "image/png", Width: 180, Height: 180, Alpha: &yes, Source: "apple-touch-icon"}
	bigOpaque := Candidate{Mime: "image/png", Width: 512, Height: 512, Alpha: &no, Source: "manifest"}
	small := Candidate{Mime: "image/png", Width: 32, Height: 32, Alpha: &yes, Source: "google"}
	ico := Candidate{Mime: "image/x-icon", Source: "favicon-ico"}

	// 质量档（矢量 / 大且透明）压过"来路更正但只是不透明位图"的候选
	pairs := []struct {
		better, worse Candidate
		why           string
	}{
		{vector, bigOpaque, "矢量优先于 512 不透明位图"},
		{bigAlpha, bigOpaque, "180 透明底优先于 512 不透明位图"},
		{bigOpaque, small, "512 优先于 32px"},
		{small, ico, "32px 位图优先于尺寸未知的 ICO"},
	}
	for _, p := range pairs {
		if candidateScore(p.better) <= candidateScore(p.worse) {
			t.Errorf("%s：%s(%d) 应高于 %s(%d)", p.why,
				p.better.Source, candidateScore(p.better), p.worse.Source, candidateScore(p.worse))
		}
	}

	// 同尺寸同时，站点自己声明的应当比第三方服务更靠前
	sameTouch := Candidate{Mime: "image/png", Width: 180, Height: 180, Alpha: &no, Source: "apple-touch-icon"}
	sameGoogle := Candidate{Mime: "image/png", Width: 180, Height: 180, Alpha: &no, Source: "google"}
	if candidateScore(sameTouch) <= candidateScore(sameGoogle) {
		t.Error("同尺寸同时，站点自述应当优先于第三方服务")
	}
}

func TestManifestIconLinksResolvesAndOrders(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/app.webmanifest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		_, _ = w.Write([]byte(`{"icons":[
			{"src":"/mask-512.png","sizes":"512x512","purpose":"maskable"},
			{"src":"icons/small-192.png","sizes":"192x192"},
			{"src":"/big-512.png","sizes":"512x512"}
		]}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	site := httptest.NewServer(mux)
	defer site.Close()

	f := NewFetcherWithEndpoints(site.URL, site.URL)
	body := []byte(`<html><head><link rel="manifest" href="app.webmanifest"></head></html>`)
	got := manifestIconLinks(context.Background(), f, body, site.URL)

	want := []string{site.URL + "/big-512.png", site.URL + "/icons/small-192.png", site.URL + "/mask-512.png"}
	if len(got) != len(want) {
		t.Fatalf("got %d urls (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("url[%d] = %s, want %s（大图在前、maskable 最后、相对路径以 manifest 地址为基准）", i, got[i], want[i])
		}
	}
}

// candidatesSite 起一个"典型现代站点"：声明 apple-touch-icon、manifest（512 大图）、
// favicon.ico（与 apple-touch 内容相同 → 应当被内容去重），并提供一台假 Google/DDG。
func candidatesSite(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	touch := makeAlphaPNG(t, 180)
	big := makePNG(t, 512)
	hits := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hits++
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head>
			<link rel="apple-touch-icon" sizes="180x180" href="/touch.png">
			<link rel="manifest" href="/app.webmanifest">
		</head></html>`))
	})
	mux.HandleFunc("/touch.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(touch)
	})
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, _ *http.Request) {
		// 与 apple-touch 同一张图：候选里只应出现一次
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(touch)
	})
	mux.HandleFunc("/app.webmanifest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		_, _ = w.Write([]byte(`{"icons":[{"src":"/big.png","sizes":"512x512","type":"image/png"}]}`))
	})
	mux.HandleFunc("/big.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(big)
	})
	site := httptest.NewServer(mux)
	t.Cleanup(site.Close)
	return site, &hits
}

func TestCandidatesCollectsSourcesAndDedupes(t *testing.T) {
	site, _ := candidatesSite(t)
	miss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound) // google / ddg 都不给东西
	}))
	defer miss.Close()

	f := NewFetcherWithEndpoints(miss.URL, miss.URL)
	got, err := f.Candidates(context.Background(), site.URL, DefaultCandidateLimit)
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("候选数 = %d, want 2（apple-touch 与 ico 内容相同应去重）: %+v", len(got), got)
	}
	if got[0].Source != "apple-touch-icon" || got[0].Width != 180 {
		t.Errorf("首个候选 = %s %dx%d, want apple-touch-icon 180x180", got[0].Source, got[0].Width, got[0].Height)
	}
	if got[0].Alpha == nil || !*got[0].Alpha {
		t.Error("apple-touch 那张是透明底，alpha 应为 true")
	}
	if got[1].Source != "manifest" || got[1].Width != 512 {
		t.Errorf("第二个候选 = %s %dx%d, want manifest 512x512", got[1].Source, got[1].Width, got[1].Height)
	}
	if len(got[0].Data) == 0 || len(got[1].Data) == 0 {
		t.Error("候选必须带上字节（调用方要直接落盘）")
	}
}

func TestCandidatesEmptySiteReturnsNoIcon(t *testing.T) {
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>nothing</title></head></html>`))
	}))
	defer empty.Close()

	f := NewFetcherWithEndpoints(empty.URL, empty.URL)
	if _, err := f.Candidates(context.Background(), empty.URL, DefaultCandidateLimit); !errors.Is(err, ErrNoIcon) {
		t.Errorf("err = %v, want ErrNoIcon", err)
	}
}

func TestCandidatesRejectsNonHTTP(t *testing.T) {
	f := NewFetcherWithEndpoints("http://127.0.0.1:1", "http://127.0.0.1:1")
	for _, raw := range []string{"", "不是网址", "ftp://example.com", "file:///etc/passwd"} {
		if _, err := f.Candidates(context.Background(), raw, 3); !errors.Is(err, ErrNoIcon) {
			t.Errorf("Candidates(%q) err = %v, want ErrNoIcon", raw, err)
		}
	}
}

func TestMaxSizeFromSizes(t *testing.T) {
	cases := map[string]int{
		"192x192":           192,
		"16x16 32x32 48x48": 48,
		"512x512":           512,
		"any":               0,
		"":                  0,
		"192x192 512x512":   512,
		"not-a-size":        0,
		"1024x1024 512x512": 1024,
	}
	for in, want := range cases {
		if got := maxSizeFromSizes(in); got != want {
			t.Errorf("maxSizeFromSizes(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestIconCandidateLinksCarrySource(t *testing.T) {
	body := []byte(`<html><head>
		<link rel="apple-touch-icon" href="/touch.png">
		<link rel="mask-icon" href="/mask.svg">
		<link rel="icon" href="/icon.png">
	</head></html>`)
	got := iconCandidateLinks(body, "https://example.com")
	if len(got) != 3 {
		t.Fatalf("got %d candidates, want 3", len(got))
	}
	sources := map[string]bool{}
	for _, c := range got {
		sources[c.source] = true
	}
	for _, want := range []string{"apple-touch-icon", "mask-icon", "link-icon"} {
		if !sources[want] {
			t.Errorf("缺少出处标签 %s：%+v", want, got)
		}
	}
	if strings.Contains(got[0].href, "NONE") {
		t.Error("unreachable")
	}
}
