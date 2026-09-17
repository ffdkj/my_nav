package nav

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ffdkj/my_nav/internal/db/dbgen"
	"github.com/ffdkj/my_nav/internal/favicon"
)

// iconCandidatesTimeout 是候选查询的总预算。
//
// 比自动抓取的 3s 宽松：这是用户**主动**输入网址后发起的查询，多等一会儿
// 换"多几张可用图标"是划算的；而自动抓取卡在"添加图标"的同步请求里，必须短。
const iconCandidatesTimeout = 8 * time.Second

// iconCandCacheTTL 是"同一个 URL 短时间内不重复抓"的窗口：
// 输错一个字再改回来、或者删掉又重加同一个站，都不必再去打对方服务器。
const iconCandCacheTTL = 60 * time.Second

// iconCandCacheMax 是缓存条数上限（个人规模够用；满了就整体清空，避免无界增长）。
const iconCandCacheMax = 64

type iconCandCacheEntry struct {
	at   time.Time
	data *IconCandidatesDTO
}

// IconCandidateDTO 是候选图标的 JSON 形态。
//
// IconPath 指向**已经落盘**的那份字节（/icons/<icon_path>），前端直接拿它当缩略图，
// 选中时也只需要把这个路径回传（见 PickIcon），不必重新下载。
type IconCandidateDTO struct {
	IconPath string `json:"icon_path"`
	Source   string `json:"source"`
	Mime     string `json:"mime"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	// Alpha: true=透明底，false=不透明，null=判不了（SVG / ICO）
	Alpha  *bool  `json:"alpha"`
	Bytes  int    `json:"bytes"`
	Remote string `json:"remote_url"`
}

type IconCandidatesDTO struct {
	URL        string             `json:"url"`
	Candidates []IconCandidateDTO `json:"candidates"`
}

// IconCandidates 抓取某个网址的候选图标（最多 6 张），并把字节直接存进
// 内容寻址仓库（去重、不可变），返回可预览的 icon_path。
//
// 不依赖 links 行：新增链接时（link 还不存在）也能先让用户挑。
// 找不到任何图标时返回**空列表 + 200**（不是错误）：UI 要显示"只有纯色兜底"的提示。
func (s *Service) IconCandidates(ctx context.Context, rawURL string) (*IconCandidatesDTO, error) {
	target := strings.TrimSpace(rawURL)
	if !validHTTPURL(target) {
		return nil, BadRequest("url must be an absolute http(s) URL")
	}
	if s.Fetcher == nil || s.Icons == nil {
		return nil, &Error{Status: http.StatusServiceUnavailable, Code: "icons_disabled", Message: "icon fetching is not configured"}
	}

	if cached := s.cachedCandidates(target); cached != nil {
		return cached, nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, iconCandidatesTimeout)
	defer cancel()

	found, err := s.Fetcher.Candidates(fetchCtx, target, favicon.DefaultCandidateLimit)
	if err != nil && !errors.Is(err, favicon.ErrNoIcon) && s.Log != nil {
		s.Log.Debug("icon candidates failed", "url", target, "err", err)
	}

	out := &IconCandidatesDTO{URL: target, Candidates: make([]IconCandidateDTO, 0, len(found))}
	for _, c := range found {
		rel, saveErr := s.Icons.Save(c.Data, c.Mime)
		if saveErr != nil {
			if s.Log != nil {
				s.Log.Warn("store icon candidate", "url", target, "err", saveErr)
			}
			continue
		}
		out.Candidates = append(out.Candidates, IconCandidateDTO{
			IconPath: rel,
			Source:   c.Source,
			Mime:     c.Mime,
			Width:    c.Width,
			Height:   c.Height,
			Alpha:    c.Alpha,
			Bytes:    len(c.Data),
			Remote:   c.Remote,
		})
	}
	s.rememberCandidates(target, out)
	return out, nil
}

// PickIconInput 是"选中某个候选"的请求体。
type PickIconInput struct {
	IconPath string `json:"icon_path"`
	// RemoteURL 是这张图的远程地址（候选接口给的 remote_url），可为空。
	// 存进 links.icon_picked_url：用来区分"用户手选的"和"自动抓的"。
	RemoteURL string `json:"remote_url"`
}

// PickIcon 把一张**已经在仓库里**的图标设为该链接的图标。
//
// 为什么不收字节、只收路径：候选接口存的就是最终要用的那份（内容寻址、不可变），
// 所以这里只校验路径能安全解析、并重新 Inspect 一次拿到真实的 mime/尺寸，
// 避免前端报上来的元数据与服务端不一致（也顺手挡住了越权路径）。
func (s *Service) PickIcon(ctx context.Context, linkID string, in PickIconInput) (*LinkDTO, error) {
	if s.Icons == nil {
		return nil, &Error{Status: http.StatusServiceUnavailable, Code: "icons_disabled", Message: "icon storage is not configured"}
	}
	if _, err := s.Q.GetLink(ctx, linkID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("link")
		}
		return nil, fmt.Errorf("get link: %w", err)
	}
	rel := strings.TrimSpace(in.IconPath)
	full, err := s.Icons.Resolve(rel)
	if err != nil {
		return nil, BadRequest("unknown icon_path")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, BadRequest("icon file is missing")
	}
	mime, w, h, err := favicon.Inspect(data)
	if err != nil {
		return nil, BadRequest("icon file is not a usable image")
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	ww, hh := int64(w), int64(h)
	var picked *string
	if remote := strings.TrimSpace(in.RemoteURL); remote != "" && validHTTPURL(remote) {
		picked = &remote
	}

	if err := s.Q.SetPickedIcon(ctx, dbgen.SetPickedIconParams{
		IconPath:      &rel,
		IconMime:      &mime,
		IconW:         &ww,
		IconH:         &hh,
		IconPickedUrl: picked,
		IconCheckedAt: &now,
		ID:            linkID,
	}); err != nil {
		return nil, fmt.Errorf("set picked icon: %w", err)
	}

	updated, err := s.Q.GetLink(ctx, linkID)
	if err != nil {
		return nil, fmt.Errorf("reload link: %w", err)
	}
	dto := toLinkDTO(updated)
	return &dto, nil
}

func (s *Service) cachedCandidates(url string) *IconCandidatesDTO {
	s.iconCandMu.Lock()
	defer s.iconCandMu.Unlock()
	entry, ok := s.iconCandCache[url]
	if !ok || time.Since(entry.at) > iconCandCacheTTL {
		return nil
	}
	return entry.data
}

func (s *Service) rememberCandidates(url string, dto *IconCandidatesDTO) {
	s.iconCandMu.Lock()
	defer s.iconCandMu.Unlock()
	if s.iconCandCache == nil || len(s.iconCandCache) >= iconCandCacheMax {
		s.iconCandCache = make(map[string]iconCandCacheEntry, iconCandCacheMax)
	}
	s.iconCandCache[url] = iconCandCacheEntry{at: time.Now(), data: dto}
}
