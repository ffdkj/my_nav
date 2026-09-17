package favicon

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

// DefaultCandidateLimit 是候选接口默认返回几个图标。
// 6 是"一屏能放下、又足够挑"的数量（前端是一排缩略图）。
const DefaultCandidateLimit = 6

// Candidate 是"能拿给用户挑一个"的图标：字节 + 元数据 + 出处。
//
// 与 Result 的区别：Result 是自动链路里"选中的那一张"，Candidate 是候选项，
// 多带了 Alpha（透明底判定，nil = 判不了）与 Remote（远程地址，用于溯源）。
type Candidate struct {
	Data   []byte
	Mime   string
	Width  int
	Height int
	Alpha  *bool
	// Source ∈ apple-touch-icon | link-icon | mask-icon | manifest | favicon-ico | google | duckduckgo
	Source string
	// Remote 是这张图的实际下载地址（Google/DDG 那种由我们构造的地址也记下来，
	// 内联 data URL 记 "inline"）
	Remote string
}

// Candidates 抓一个站点的**多张**候选图标，按"更适合当 logo"排序后返回（最多 limit 张）。
//
// 来源（Q7 决策，全部保留原有的自动链路不动）：
//  1. 站点自述：<link rel="...icon...">（apple-touch-icon / SVG / 各 sizes 的 PNG）
//  2. web app manifest 里的 icons[]
//  3. /favicon.ico
//  4. Google faviconV2（size=256，比自动链路的 128 大）
//  5. DuckDuckGo
//
// 内容相同的候选（同一张图被多个来源声明）只保留一份。
// 抓取是并发的（并发度与 fetcher 的信号量一致）。
func (f *Fetcher) Candidates(ctx context.Context, pageURL string, limit int) ([]Candidate, error) {
	if limit <= 0 {
		limit = DefaultCandidateLimit
	}
	target, err := url.Parse(strings.TrimSpace(pageURL))
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, ErrNoIcon
	}
	origin := target.Scheme + "://" + target.Host

	type task struct {
		run func(context.Context) (Candidate, error)
	}
	var tasks []task

	// 站点自述：HTML 只取一次，<link> 与 manifest 共用
	body, _ := f.getHTML(ctx, origin)
	if len(body) > 0 {
		for _, c := range iconCandidateLinks(body, origin) {
			href, source := c.href, c.source
			tasks = append(tasks, task{run: func(ctx context.Context) (Candidate, error) {
				if strings.HasPrefix(href, "data:") {
					// 内联 data URL：不需要联网，直接解码（自动链路里反而用不上它，
					// 因为 http.NewRequest 不接受 data:）
					data, mime, ok := DecodeDataURL(href)
					if !ok {
						return Candidate{}, ErrUnknownImage
					}
					r, err := f.result(data, mime, source)
					if err != nil {
						return Candidate{}, err
					}
					return candidateFromResult(r, source, "inline"), nil
				}
				return f.fetchCandidate(ctx, href, source)
			}})
		}
		for _, href := range manifestIconLinks(ctx, f, body, origin) {
			url := href
			tasks = append(tasks, task{run: func(ctx context.Context) (Candidate, error) {
				return f.fetchCandidate(ctx, url, "manifest")
			}})
		}
	}
	icoURL := origin + "/favicon.ico"
	tasks = append(tasks, task{run: func(ctx context.Context) (Candidate, error) {
		return f.fetchCandidate(ctx, icoURL, "favicon-ico")
	}})

	// 两家外部服务：与站点抓取并行，各自仍套 700ms 的单步限时，
	// 保证它们不会把总预算吃光（同 probe 的理由）。
	googleURL := f.googleURL(pageURL, candidateGoogleSize)
	tasks = append(tasks, task{run: func(ctx context.Context) (Candidate, error) {
		r, err := f.probe(ctx, func(c context.Context) (*Result, error) { return f.google(c, pageURL) })
		if err != nil {
			return Candidate{}, err
		}
		return candidateFromResult(r, "google", googleURL), nil
	}})
	ddgURL := f.ddgURL(target.Host)
	tasks = append(tasks, task{run: func(ctx context.Context) (Candidate, error) {
		r, err := f.probe(ctx, func(c context.Context) (*Result, error) { return f.ddg(c, target.Host) })
		if err != nil {
			return Candidate{}, err
		}
		return candidateFromResult(r, "duckduckgo", ddgURL), nil
	}})

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	const workers = 4 // 与 fetcher 的信号量一致
	sem := make(chan struct{}, workers)
	results := make(chan Candidate, len(tasks))
	var wg sync.WaitGroup
	for _, t := range tasks {
		wg.Add(1)
		go func(t task) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-cctx.Done():
				return
			}
			defer func() { <-sem }()
			if c, err := t.run(cctx); err == nil && len(c.Data) > 0 {
				results <- c
			}
		}(t)
	}
	go func() { wg.Wait(); close(results) }()

	// 先按"适不适合当 logo"排序，**再去重**：同一张图常常被多个来源声明
	// （apple-touch-icon 与 /favicon.ico 往往就是同一份字节），
	// 去重必须让分数最高的那个来源活下来 —— 反过来的话，谁先下载完谁留下，
	// 结果就是"出处"变成了随机的。
	var all []Candidate
	for c := range results {
		all = append(all, c)
	}
	sort.SliceStable(all, func(i, j int) bool {
		return candidateScore(all[i]) > candidateScore(all[j])
	})

	picked := make([]Candidate, 0, len(all))
	seen := make(map[[32]byte]struct{}, len(all))
	for _, c := range all {
		sum := sha256.Sum256(c.Data)
		if _, dup := seen[sum]; dup {
			continue
		}
		seen[sum] = struct{}{}
		picked = append(picked, c)
		if len(picked) == limit {
			break
		}
	}
	if len(picked) == 0 {
		return nil, ErrNoIcon
	}
	return picked, nil
}

// candidateGoogleSize 是候选查询向 faviconV2 要的尺寸（自动链路用 128）。
const candidateGoogleSize = 256

func (f *Fetcher) fetchCandidate(ctx context.Context, raw, source string) (Candidate, error) {
	data, ctype, err := f.get(ctx, raw)
	if err != nil {
		return Candidate{}, err
	}
	r, err := f.result(data, ctype, source)
	if err != nil {
		return Candidate{}, err
	}
	return candidateFromResult(r, source, raw), nil
}

func candidateFromResult(r *Result, source, remote string) Candidate {
	return Candidate{
		Data:   r.Data,
		Mime:   r.Mime,
		Width:  r.Width,
		Height: r.Height,
		Alpha:  HasAlpha(r.Data),
		Source: source,
		Remote: remote,
	}
}

// candidateScore 把"更适合当 logo"编码成一个整数（越大越靠前）：
//
//	质量档（能否当高清 logo）> 来源可靠度 > 尺寸
//
// 质量档：矢量 / ≥180 且透明底 = 3；≥180 = 2；其余（含尺寸未知的 ICO）= 1。
func candidateScore(c Candidate) int {
	maxSide := c.Width
	if c.Height > maxSide {
		maxSide = c.Height
	}
	alpha := c.Alpha != nil && *c.Alpha
	vector := c.Mime == "image/svg+xml"

	tier := 1
	switch {
	case vector || (maxSide >= 180 && alpha):
		tier = 3
	case maxSide >= 180:
		tier = 2
	}

	sourceRank := map[string]int{
		"apple-touch-icon": 5,
		"manifest":         5,
		"link-icon":        4,
		"google":           3,
		"duckduckgo":       2,
		"mask-icon":        1, // 单色剪影，能当 logo 但常常只有一种颜色
		"favicon-ico":      1,
	}
	sizeBonus := 0
	if maxSide > 0 {
		sizeBonus = min(maxSide, 512) / 64
	}
	return tier*100 + sourceRank[c.Source]*10 + sizeBonus
}

// ---------- manifest ----------

type manifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose"`
}

type manifestDoc struct {
	Icons []manifestIcon `json:"icons"`
}

// manifestIconLinks 找 <link rel="manifest"> 并把 manifest 里的 icons[] 展开成 URL 列表。
//
// 之前完全没有解析 manifest —— 而现代站点（尤其 PWA）往往只在这里声明 192/512 的
// 高清 PNG，那正是"高清 logo"最靠谱的来源。
// maskable 图标排在最后：它是给系统裁切用的，四周带安全边距，直接当 logo 会显得小。
func manifestIconLinks(ctx context.Context, f *Fetcher, body []byte, origin string) []string {
	href := manifestHref(body, origin)
	if href == "" {
		return nil
	}
	raw, err := f.getHTML(ctx, href)
	if err != nil {
		return nil
	}
	var doc manifestDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	base := href // manifest 里的相对路径以 manifest 自己的地址为基准
	type entry struct {
		url  string
		size int
		mask bool
	}
	var entries []entry
	for _, ic := range doc.Icons {
		src := strings.TrimSpace(ic.Src)
		if src == "" || strings.HasPrefix(src, "data:") {
			continue
		}
		resolved, err := resolveURL(base, src)
		if err != nil {
			continue
		}
		entries = append(entries, entry{
			url:  resolved,
			size: maxSizeFromSizes(ic.Sizes),
			mask: strings.Contains(strings.ToLower(ic.Purpose), "maskable"),
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].mask != entries[j].mask {
			return !entries[i].mask
		}
		return entries[i].size > entries[j].size
	})
	out := make([]string, 0, len(entries))
	seen := map[string]struct{}{}
	for _, e := range entries {
		if _, dup := seen[e.url]; dup {
			continue
		}
		seen[e.url] = struct{}{}
		out = append(out, e.url)
	}
	return out
}

// manifestHref 从 HTML 里取 <link rel="manifest" href="..."> 的绝对地址。
func manifestHref(body []byte, origin string) string {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return ""
	}
	base := origin + "/"
	if b := findBase(doc); b != "" {
		if resolved, err := resolveURL(base, b); err == nil {
			base = resolved
		}
	}
	var found string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "link" {
			var rel, href string
			for _, a := range n.Attr {
				switch strings.ToLower(a.Key) {
				case "rel":
					rel = strings.ToLower(a.Val)
				case "href":
					href = strings.TrimSpace(a.Val)
				}
			}
			if href != "" && strings.Contains(rel, "manifest") {
				if resolved, err := resolveURL(base, href); err == nil {
					found = resolved
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

// maxSizeFromSizes 解析 "192x192 512x512" 里的最大边长（"any" 之类返回 0）。
func maxSizeFromSizes(sizes string) int {
	best := 0
	for _, part := range strings.Fields(strings.ToLower(sizes)) {
		dims := strings.SplitN(part, "x", 2)
		if len(dims) != 2 {
			continue
		}
		if v, err := strconv.Atoi(dims[0]); err == nil && v > best {
			best = v
		}
	}
	return best
}
