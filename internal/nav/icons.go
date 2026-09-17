package nav

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ffdkj/my_nav/internal/db/dbgen"
	"github.com/ffdkj/my_nav/internal/favicon"
)

// iconFetchTimeout 是单次抓取的总预算。超过就当作"没抓到"，
// 不能让一个慢站点把"添加图标"这个操作卡住。
const iconFetchTimeout = 3 * time.Second

// uploadMaxSide 是上传图标归一化后的最长边。
const uploadMaxSide = 256

// refreshIconsForPage 为该页所有还处于 pending 的链接抓一次图标。
//
// 调用时机很关键：必须在**事务提交之后**。抓取是网络 I/O（最长 3s），
// 放在事务里会把 SQLite 的写锁一直握着，整站写操作全被拖住。
func (s *Service) refreshIconsForPage(ctx context.Context, pageID string) {
	if s.Fetcher == nil || s.Icons == nil {
		return
	}
	links, err := s.Q.ListLinksForPage(ctx, pageID)
	if err != nil {
		return
	}
	for _, link := range links {
		if link.IconStatus != "pending" || link.IconSource != "auto" {
			continue
		}
		if _, err := s.FetchAndStoreIcon(ctx, link.ID); err != nil && s.Log != nil {
			s.Log.Debug("icon fetch failed", "link", link.ID, "url", link.Url, "err", err)
		}
	}
}

// FetchAndStoreIcon 走完整链路抓取并落库；ErrNoIcon 会记为 miss（前端显示纯色兜底）。
func (s *Service) FetchAndStoreIcon(ctx context.Context, linkID string) (*LinkDTO, error) {
	link, err := s.Q.GetLink(ctx, linkID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("link")
		}
		return nil, fmt.Errorf("get link: %w", err)
	}
	if s.Fetcher == nil || s.Icons == nil {
		return nil, &Error{Status: http.StatusServiceUnavailable, Code: "icons_disabled", Message: "icon fetching is not configured"}
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	fetchCtx, cancel := context.WithTimeout(ctx, iconFetchTimeout)
	defer cancel()

	result, fetchErr := s.Fetcher.Fetch(fetchCtx, link.Url)

	switch {
	case fetchErr == nil:
		rel, err := s.Icons.Save(result.Data, result.Mime)
		if err != nil {
			return nil, fmt.Errorf("store icon: %w", err)
		}
		link.IconPath = &rel
		link.IconMime = &result.Mime
		w, h := int64(result.Width), int64(result.Height)
		link.IconW, link.IconH = &w, &h
		link.IconStatus = "ok"
		link.IconCheckedAt = &now
		link.IconSource = "auto"

	case errors.Is(fetchErr, favicon.ErrNoIcon):
		// 负结果也要记时间：30 天内不再自动重试，避免每次打开都在白跑
		link.IconPath = nil
		link.IconMime = nil
		link.IconW, link.IconH = nil, nil
		link.IconStatus = "miss"
		link.IconCheckedAt = &now
		link.IconSource = "auto"

	default:
		link.IconStatus = "error"
		link.IconCheckedAt = &now
	}

	if err := s.Q.UpdateLink(ctx, dbgen.UpdateLinkParams{
		Title:         link.Title,
		Url:           link.Url,
		OpenNewTab:    link.OpenNewTab,
		IconSource:    link.IconSource,
		IconPath:      link.IconPath,
		IconMime:      link.IconMime,
		IconW:         link.IconW,
		IconH:         link.IconH,
		IconStatus:    link.IconStatus,
		IconCheckedAt: link.IconCheckedAt,
		IconPickedUrl: nil, // 自动重抓 = 覆盖手选（reset/refetch 都是用户主动要重抓）
		MonoText:      link.MonoText,
		MonoColor:     link.MonoColor,
		MonoFontSize:  link.MonoFontSize,
		ID:            link.ID,
	}); err != nil {
		return nil, fmt.Errorf("update link icon: %w", err)
	}

	if fetchErr != nil && !errors.Is(fetchErr, favicon.ErrNoIcon) && s.Log != nil {
		s.Log.Warn("icon fetch error", "link", link.ID, "url", link.Url, "err", fetchErr)
	}

	updated, err := s.Q.GetLink(ctx, linkID)
	if err != nil {
		return nil, fmt.Errorf("reload link: %w", err)
	}
	dto := toLinkDTO(updated)
	return &dto, nil
}

type MonogramInput struct {
	Text     string `json:"text"`
	Color    string `json:"color"`
	FontSize int64  `json:"font_size"`
}

// SetMonogram 切到纯色文字图标（抓不到图标时的兜底形态）。
func (s *Service) SetMonogram(ctx context.Context, linkID string, in MonogramInput) (*LinkDTO, error) {
	link, err := s.Q.GetLink(ctx, linkID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("link")
		}
		return nil, fmt.Errorf("get link: %w", err)
	}
	if in.Color != "" && !validHexColor(in.Color) {
		return nil, BadRequest("color must be a #rrggbb value")
	}
	if in.FontSize < 0 || in.FontSize > 200 {
		return nil, BadRequest("font_size must be between 0 and 200")
	}

	link.IconSource = "monogram"
	link.IconPath = nil
	link.IconMime = nil
	link.IconW, link.IconH = nil, nil
	link.IconStatus = "miss"
	if in.Text != "" {
		text := in.Text
		link.MonoText = &text
	}
	if in.Color != "" {
		link.MonoColor = in.Color
	}
	if in.FontSize > 0 {
		link.MonoFontSize = in.FontSize
	}

	if err := s.Q.UpdateLink(ctx, dbgen.UpdateLinkParams{
		Title:         link.Title,
		Url:           link.Url,
		OpenNewTab:    link.OpenNewTab,
		IconSource:    link.IconSource,
		IconPath:      link.IconPath,
		IconMime:      link.IconMime,
		IconW:         link.IconW,
		IconH:         link.IconH,
		IconStatus:    link.IconStatus,
		IconCheckedAt: link.IconCheckedAt,
		IconPickedUrl: nil, // 自动重抓 = 覆盖手选（reset/refetch 都是用户主动要重抓）
		MonoText:      link.MonoText,
		MonoColor:     link.MonoColor,
		MonoFontSize:  link.MonoFontSize,
		ID:            link.ID,
	}); err != nil {
		return nil, fmt.Errorf("update link: %w", err)
	}
	updated, err := s.Q.GetLink(ctx, linkID)
	if err != nil {
		return nil, fmt.Errorf("reload link: %w", err)
	}
	dto := toLinkDTO(updated)
	return &dto, nil
}

// StoreUploadedIcon 处理本地上传：归一化成 ≤256px 的 PNG（SVG/ICO 原样保存）。
func (s *Service) StoreUploadedIcon(ctx context.Context, linkID string, data []byte) (*LinkDTO, error) {
	if s.Icons == nil {
		return nil, &Error{Status: http.StatusServiceUnavailable, Code: "icons_disabled", Message: "icon storage is not configured"}
	}
	link, err := s.Q.GetLink(ctx, linkID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("link")
		}
		return nil, fmt.Errorf("get link: %w", err)
	}

	normalized, mime, w, h, err := favicon.NormalizeUpload(data, uploadMaxSide)
	if err != nil {
		return nil, BadRequest("unsupported image: " + err.Error())
	}
	rel, err := s.Icons.Save(normalized, mime)
	if err != nil {
		return nil, fmt.Errorf("store upload: %w", err)
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	link.IconSource = "upload"
	link.IconPath = &rel
	link.IconMime = &mime
	ww, hh := int64(w), int64(h)
	link.IconW, link.IconH = &ww, &hh
	link.IconStatus = "ok"
	link.IconCheckedAt = &now

	if err := s.Q.UpdateLink(ctx, dbgen.UpdateLinkParams{
		Title:         link.Title,
		Url:           link.Url,
		OpenNewTab:    link.OpenNewTab,
		IconSource:    link.IconSource,
		IconPath:      link.IconPath,
		IconMime:      link.IconMime,
		IconW:         link.IconW,
		IconH:         link.IconH,
		IconStatus:    link.IconStatus,
		IconCheckedAt: link.IconCheckedAt,
		IconPickedUrl: nil, // 自动重抓 = 覆盖手选（reset/refetch 都是用户主动要重抓）
		MonoText:      link.MonoText,
		MonoColor:     link.MonoColor,
		MonoFontSize:  link.MonoFontSize,
		ID:            link.ID,
	}); err != nil {
		return nil, fmt.Errorf("update link: %w", err)
	}
	updated, err := s.Q.GetLink(ctx, linkID)
	if err != nil {
		return nil, fmt.Errorf("reload link: %w", err)
	}
	dto := toLinkDTO(updated)
	return &dto, nil
}

// ResetIcon 回到"标准 favicon"并重新抓取。
func (s *Service) ResetIcon(ctx context.Context, linkID string) (*LinkDTO, error) {
	if _, err := s.Q.GetLink(ctx, linkID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("link")
		}
		return nil, fmt.Errorf("get link: %w", err)
	}
	if err := s.Q.ClearLinkIcon(ctx, linkID); err != nil {
		return nil, fmt.Errorf("clear icon: %w", err)
	}
	return s.FetchAndStoreIcon(ctx, linkID)
}

func validHexColor(v string) bool {
	if len(v) != 7 || !strings.HasPrefix(v, "#") {
		return false
	}
	for _, r := range v[1:] {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// IconFile 为 /icons/... 路由解析出磁盘路径。
func (s *Service) IconFile(rel string) (string, error) {
	if s.Icons == nil {
		return "", &Error{Status: http.StatusServiceUnavailable, Code: "icons_disabled", Message: "icon storage is not configured"}
	}
	full, err := s.Icons.Resolve(rel)
	if err != nil {
		return "", NotFound("icon")
	}
	return full, nil
}
