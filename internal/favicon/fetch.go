// Package favicon 实现图标抓取链与内容寻址存储。
//
// 链路（优先级从高到低，来源见 docs/research/04-favicon-caddy-tailscale.md，均为实测）：
//  1. Google `faviconV2` —— **直连**，不走 www.google.com/s2/favicons：
//     后者只是 301 跳板、响应体是 HTML（不跟随重定向就会把 HTML 当图标存下来），
//     且跳转后 Host 在 t0~t3 之间轮换。无图标时它返回 **404**（body 是固定的地球 PNG 哨兵）。
//  2. DuckDuckGo `icons.duckduckgo.com/ip3/<host>.ico`，同样用状态码判失败。
//  3. 自建发现：抓首页 HTML 解析 <link rel="icon"> 等，再兜 /favicon.ico。
//  4. 全失败 → ErrNoIcon，由上层标记 icon_status=miss（前端显示纯色文字兜底）。
//
// 注意 `sz` 只是**提示**：站点只提供 16/24/32 时，要 128 也只会拿到 32。
// 所以第 1 步结果偏小时会继续尝试第 3 步，拿不到再退回第 1 步的结果。
package favicon

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// ErrNoIcon 表示整条链路都认为"这个站点没有图标"（与网络错误区分开）。
var ErrNoIcon = errors.New("favicon: no icon found")

const userAgent = "my_nav/1.0 (+personal navigator)"

// smallIconPx 以下认为"太小，值得再做一次自建发现"。
const smallIconPx = 64

// probeTimeout 是"外部图标服务"（Google / DuckDuckGo）单步的时间上限，
// 目的是给**自建发现**留出预算，见 (*Fetcher).probe 的说明。
const probeTimeout = 700 * time.Millisecond

type Result struct {
	Data   []byte
	Mime   string
	Width  int
	Height int
	Source string // google | duckduckgo | discovered
}

type Fetcher struct {
	googleBase   string
	ddgBase      string
	size         int
	allowPrivate bool
	client       *http.Client
	sem          chan struct{}
	log          *slog.Logger
}

// NewFetcher 构造抓取器。allowPrivate=false 时启用 SSRF 防护（默认）。
func NewFetcher(log *slog.Logger, allowPrivate bool) *Fetcher {
	return &Fetcher{
		googleBase:   "https://t2.gstatic.com/faviconV2",
		ddgBase:      "https://icons.duckduckgo.com/ip3",
		size:         128,
		allowPrivate: allowPrivate,
		client:       newClient(4*time.Second, allowPrivate),
		sem:          make(chan struct{}, 4),
		log:          log,
	}
}

// NewFetcherWithEndpoints 供测试注入 httptest 端点，并放开私网限制。
func NewFetcherWithEndpoints(googleBase, ddgBase string) *Fetcher {
	f := NewFetcher(nil, true)
	f.googleBase = googleBase
	f.ddgBase = ddgBase
	return f
}

// Fetch 按链路取回一个图标。
func (f *Fetcher) Fetch(ctx context.Context, pageURL string) (*Result, error) {
	target, err := url.Parse(strings.TrimSpace(pageURL))
	if err != nil || target.Host == "" {
		return nil, ErrNoIcon
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, ErrNoIcon
	}
	origin := target.Scheme + "://" + target.Host

	// 1) Google
	googleResult, googleErr := f.probe(ctx, func(c context.Context) (*Result, error) {
		return f.google(c, pageURL)
	})
	if googleErr == nil && googleResult.Width >= smallIconPx {
		return googleResult, nil
	}

	// 2) DuckDuckGo
	if r, err := f.probe(ctx, func(c context.Context) (*Result, error) {
		return f.ddg(c, target.Host)
	}); err == nil && r.Width >= smallIconPx {
		return r, nil
	}

	// 3) 自建发现（能拿到 apple-touch-icon 这类高清源）
	if r, err := f.discover(ctx, origin); err == nil {
		return r, nil
	}

	// 4) 退回第 1/2 步的小图标；都没有就是真没有
	if googleErr == nil {
		return googleResult, nil
	}
	if r, err := f.ddg(ctx, target.Host); err == nil {
		return r, nil
	}
	if f.log != nil {
		f.log.Debug("favicon miss", "url", pageURL)
	}
	return nil, ErrNoIcon
}

// probe 给"外部图标服务"（Google / DuckDuckGo）套一层单步限时。
//
// 为什么必须有：总预算只有 3s（nav.iconFetchTimeout），而这两步都是外网请求。
// 实测 DuckDuckGo 会偶发 TLS 重置并一路挂到 5s —— 预算被它吃光后，
// **自建发现**（第 3 步，也是本地/局域网站点唯一的出路）连一个请求都发不出去，
// 于是"首页明明声明了 apple-touch-icon"却记成 miss。
// 单步限时之后，第 3 步总能拿到剩下的 1.5s 以上。
func (f *Fetcher) probe(ctx context.Context, fn func(context.Context) (*Result, error)) (*Result, error) {
	cctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	return fn(cctx)
}

func (f *Fetcher) google(ctx context.Context, pageURL string) (*Result, error) {
	q := url.Values{}
	q.Set("client", "SOCIAL")
	q.Set("type", "FAVICON")
	q.Set("fallback_opts", "TYPE,SIZE,URL")
	q.Set("url", pageURL) // 必须带 scheme，否则 faviconV2 返回 404
	q.Set("size", strconv.Itoa(f.size))

	data, ctype, err := f.get(ctx, f.googleBase+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	return f.result(data, ctype, "google")
}

func (f *Fetcher) ddg(ctx context.Context, host string) (*Result, error) {
	data, ctype, err := f.get(ctx, f.ddgBase+"/"+url.PathEscape(host)+".ico")
	if err != nil {
		return nil, err
	}
	return f.result(data, ctype, "duckduckgo")
}

func (f *Fetcher) discover(ctx context.Context, origin string) (*Result, error) {
	if body, err := f.getHTML(ctx, origin); err == nil {
		for _, candidate := range iconCandidates(body, origin) {
			if r, err := f.tryURL(ctx, candidate); err == nil {
				r.Source = "discovered"
				return r, nil
			}
		}
	}
	if r, err := f.tryURL(ctx, origin+"/favicon.ico"); err == nil {
		r.Source = "discovered"
		return r, nil
	}
	return nil, ErrNoIcon
}

func (f *Fetcher) tryURL(ctx context.Context, raw string) (*Result, error) {
	data, ctype, err := f.get(ctx, raw)
	if err != nil {
		return nil, err
	}
	return f.result(data, ctype, "discovered")
}

func (f *Fetcher) result(data []byte, contentType, source string) (*Result, error) {
	mime, w, h, err := Inspect(data)
	if err != nil {
		return nil, err
	}
	// Content-Type 只在它能给出更具体的信息时才采信（有些站点一律回 text/plain）
	if contentType != "" && !strings.Contains(contentType, "text/plain") {
		ct := strings.TrimSpace(strings.Split(contentType, ";")[0])
		if strings.HasPrefix(ct, "image/") && ct != mime {
			mime = ct
		}
	}
	// data: URL 形式的图标（少数站点内联）
	if strings.HasPrefix(mime, "image/") && len(data) == 0 {
		return nil, ErrNoIcon
	}
	return &Result{Data: data, Mime: mime, Width: w, Height: h, Source: source}, nil
}

func (f *Fetcher) get(ctx context.Context, rawURL string) ([]byte, string, error) {
	select {
	case f.sem <- struct{}{}:
		defer func() { <-f.sem }()
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "image/*,*/*;q=0.8")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("favicon: %s returned %d", rawURL, resp.StatusCode)
	}
	data, err := readCapped(resp.Body, maxIconBytes)
	if err != nil {
		return nil, "", err
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func (f *Fetcher) getHTML(ctx context.Context, rawURL string) ([]byte, error) {
	select {
	case f.sem <- struct{}{}:
		defer func() { <-f.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("favicon: %s returned %d", rawURL, resp.StatusCode)
	}
	// 首页可能很大，只读前面一段用于找 <link>
	return readCapped(resp.Body, 512<<10)
}

type iconCandidate struct {
	href  string
	score int
}

// iconCandidates 解析 <link rel="...icon...">，按优先级排序并解析成绝对 URL。
// 优先级参考浏览器行为：SVG 与 apple-touch-icon（通常 180x180）优于 /favicon.ico。
func iconCandidates(body []byte, origin string) []string {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil
	}
	base := origin + "/"
	if href := findBase(doc); href != "" {
		if resolved, err := resolveURL(base, href); err == nil {
			base = resolved
		}
	}

	var candidates []iconCandidate
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "link" {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[strings.ToLower(a.Key)] = a.Val
			}
			rel := strings.ToLower(attrs["rel"])
			href := strings.TrimSpace(attrs["href"])
			if href == "" || !strings.Contains(rel, "icon") {
				goto children
			}
			if strings.HasPrefix(href, "data:") {
				// 内联 data URL：直接可用，但常见于小图标，给中等优先级
				candidates = append(candidates, iconCandidate{href: href, score: 50})
				goto children
			}
			if resolved, err := resolveURL(base, href); err == nil {
				candidates = append(candidates, iconCandidate{
					href:  resolved,
					score: scoreIconLink(rel, attrs["type"], attrs["sizes"]),
				})
			}
		}
	children:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	seen := map[string]struct{}{}
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if _, dup := seen[c.href]; dup {
			continue
		}
		seen[c.href] = struct{}{}
		out = append(out, c.href)
	}
	return out
}

func scoreIconLink(rel, typ, sizes string) int {
	typeIsSVG := strings.Contains(strings.ToLower(typ), "svg")
	sizeIsAny := strings.Contains(strings.ToLower(sizes), "any")
	switch {
	case strings.Contains(rel, "mask-icon"):
		return 10
	case strings.Contains(rel, "apple-touch-icon"):
		return 70
	case strings.Contains(rel, "shortcut"):
		return 40
	default: // rel="icon"
		if typeIsSVG || sizeIsAny {
			return 100
		}
		if sizes != "" {
			// "16x16 32x32" 取最大的一档
			best := 0
			for _, part := range strings.Fields(sizes) {
				dims := strings.SplitN(strings.ToLower(part), "x", 2)
				if len(dims) != 2 {
					continue
				}
				if v, err := strconv.Atoi(dims[0]); err == nil && v > best {
					best = v
				}
			}
			if best > 0 {
				return 60 + min(best, 512)/16
			}
		}
		return 55
	}
}

func findBase(doc *html.Node) string {
	var found string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "base" {
			for _, a := range n.Attr {
				if strings.EqualFold(a.Key, "href") {
					found = strings.TrimSpace(a.Val)
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

func resolveURL(base, href string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	h, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	resolved := b.ResolveReference(h)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return "", fmt.Errorf("favicon: unsupported scheme %q", resolved.Scheme)
	}
	return resolved.String(), nil
}

// DecodeDataURL 支持 rel=icon 里的内联 data: URL。
func DecodeDataURL(raw string) ([]byte, string, bool) {
	if !strings.HasPrefix(raw, "data:") {
		return nil, "", false
	}
	rest := strings.TrimPrefix(raw, "data:")
	meta, payload, ok := strings.Cut(rest, ",")
	if !ok {
		return nil, "", false
	}
	mime := strings.SplitN(meta, ";", 2)[0]
	if strings.HasSuffix(meta, ";base64") {
		data, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, "", false
		}
		return data, mime, true
	}
	return []byte(payload), mime, true
}

// Download 用与抓取图标相同的防护策略下载一个文件（壁纸"下载到服务器"用）。
// 返回内容与 Content-Type；调用方负责校验格式。
func Download(ctx context.Context, rawURL string, maxBytes int64, allowPrivate bool) ([]byte, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return nil, "", errors.New("favicon: invalid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, "", errors.New("favicon: only http(s) URLs are supported")
	}

	client := newClient(20*time.Second, allowPrivate)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "image/*,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("favicon: %s returned %d", parsed.String(), resp.StatusCode)
	}
	data, err := readCapped(resp.Body, maxBytes)
	if err != nil {
		return nil, "", err
	}
	return data, resp.Header.Get("Content-Type"), nil
}
