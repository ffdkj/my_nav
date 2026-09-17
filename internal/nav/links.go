package nav

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ffdkj/my_nav/internal/db/dbgen"
)

// LinkWithPageDTO 是全局搜索用的扁平结构（带所属页面信息）。
// 匿名内嵌 LinkDTO 会让 JSON 字段平铺，与前端 types.ts 的 LinkWithPage 对齐。
type LinkWithPageDTO struct {
	LinkDTO
	PageID   string `json:"page_id"`
	PageSlug string `json:"page_slug"`
	PageName string `json:"page_name"`
}

// ListAllLinks 返回全部链接及其所属页面，供前端做全局模糊搜索。
// 同一个链接若被放在两个页面，会出现两行（各自带页面信息）——这是刻意的。
func (s *Service) ListAllLinks(ctx context.Context) ([]LinkWithPageDTO, error) {
	rows, err := s.Q.ListAllLinksWithPage(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all links: %w", err)
	}
	out := make([]LinkWithPageDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, LinkWithPageDTO{
			LinkDTO: LinkDTO{
				ID:           r.ID,
				Title:        r.Title,
				URL:          r.Url,
				OpenNewTab:   r.OpenNewTab != 0,
				IconSource:   r.IconSource,
				IconPath:     r.IconPath,
				IconStatus:   r.IconStatus,
				MonoText:     r.MonoText,
				MonoColor:    r.MonoColor,
				MonoFontSize: r.MonoFontSize,
				// 与 toLinkDTO 保持同步：这里漏掉过一次，表现为 /api/links 的
				// icon_mime/icon_w/icon_h 永远是 null，而 board 接口是好的
				IconMime:      r.IconMime,
				IconW:         r.IconW,
				IconH:         r.IconH,
				IconPickedURL: r.IconPickedUrl,
			},
			PageID:   r.PageID,
			PageSlug: r.PageSlug,
			PageName: r.PageName,
		})
	}
	return out, nil
}

type UpdateLinkInput struct {
	Title      *string `json:"title"`
	URL        *string `json:"url"`
	OpenNewTab *bool   `json:"open_new_tab"`
}

// UpdateLinkFields 只改元数据。若 URL 变了，图标缓存作废：
// 重置为 pending 并清空 icon_path，让 M5 的抓取链重新取一次。
func (s *Service) UpdateLinkFields(ctx context.Context, id string, in UpdateLinkInput) (*LinkDTO, error) {
	link, err := s.Q.GetLink(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("link")
		}
		return nil, fmt.Errorf("get link: %w", err)
	}

	if in.Title != nil {
		title := *in.Title
		if title == "" {
			title = hostOf(link.Url)
		}
		link.Title = title
	}
	if in.OpenNewTab != nil {
		if *in.OpenNewTab {
			link.OpenNewTab = 1
		} else {
			link.OpenNewTab = 0
		}
	}
	if in.URL != nil && *in.URL != link.Url {
		if !validHTTPURL(*in.URL) {
			return nil, BadRequest("url must be an absolute http(s) URL")
		}
		link.Url = *in.URL
		// 只有"自动抓来的图标"才随 URL 作废（spec §6）。手选（icon_picked_url）、
		// 本地上传、纯色文字都是用户明确挑过的，改个域名不该把它抹掉 ——
		// 之前这里无条件重置，于是换个网址手选的图标就悄悄没了。
		if link.IconSource == "auto" && link.IconPickedUrl == nil {
			link.IconPath = nil
			link.IconMime = nil
			link.IconW, link.IconH = nil, nil
			link.IconStatus = "pending"
			link.IconCheckedAt = nil
		}
		link.MonoColor = monogramColor(link.Url)
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
		IconPickedUrl: link.IconPickedUrl,
		MonoText:      link.MonoText,
		MonoColor:     link.MonoColor,
		MonoFontSize:  link.MonoFontSize,
		ID:            id,
	}); err != nil {
		return nil, fmt.Errorf("update link: %w", err)
	}

	updated, err := s.Q.GetLink(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reload link: %w", err)
	}
	dto := toLinkDTO(updated)
	return &dto, nil
}

// Settings 的可写白名单：避免前端手滑写入垃圾键。
var writableSettings = map[string]struct{}{
	"default_engine_id":      {},
	"merge_dwell_ms":         {},
	"page_flip_edge_ms":      {},
	"wallpaper_mode":         {},
	"wallpaper_id":           {},
	"wallpaper_rotation":     {},
	"wallpaper_interval_min": {},
	"wallpaper_fallback":     {},
	"search_open_new_tab":    {},
	"theme":                  {},
	"tile_shape":             {},
}

// tileShapes 与前端 web/src/lib/shape.ts 的 TILE_SHAPES 一一对应：
// 值只是"容器圆角"的代号，真正的像素/百分比写在前端那一边。
var tileShapes = map[string]struct{}{
	"rounded":  {},
	"circle":   {},
	"squircle": {},
	"square":   {},
}

// UpdateSettings 只做 upsert，不做整体替换（PATCH 语义）。
func (s *Service) UpdateSettings(ctx context.Context, updates map[string]string) error {
	if len(updates) == 0 {
		return BadRequest("no settings provided")
	}
	for key := range updates {
		if _, ok := writableSettings[key]; !ok {
			return BadRequest("unknown setting: " + key)
		}
	}
	if v, ok := updates["tile_shape"]; ok {
		if _, valid := tileShapes[v]; !valid {
			return BadRequest("invalid tile_shape: " + v)
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := dbgen.New(tx)
	for key, value := range updates {
		if err := q.UpsertSetting(ctx, dbgen.UpsertSettingParams{K: key, V: value}); err != nil {
			return fmt.Errorf("upsert setting %s: %w", key, err)
		}
	}
	return tx.Commit()
}
