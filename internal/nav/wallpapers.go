package nav

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ffdkj/my_nav/internal/db/dbgen"
	"github.com/ffdkj/my_nav/internal/favicon"
	"github.com/ffdkj/my_nav/internal/imageproc"
)

// WallpaperDTO 是给前端的形状。
// 前端据此拼出展示 URL：
//
//	kind=upload → /wallpapers/thumb/<thumb_file>（列表用）或 /wallpapers/orig/<file>（全屏用）
//	kind=url    → 直接使用 remote_url（省服务器流量）
type WallpaperDTO struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	RemoteURL *string `json:"remote_url"`
	File      *string `json:"file"`
	ThumbFile *string `json:"thumb_file"`
	W         *int64  `json:"w"`
	H         *int64  `json:"h"`
	Bytes     *int64  `json:"bytes"`
	SortOrder int64   `json:"sort_order"`
}

func toWallpaperDTO(w dbgen.Wallpaper) WallpaperDTO {
	return WallpaperDTO{
		ID:        w.ID,
		Kind:      w.Kind,
		RemoteURL: w.RemoteUrl,
		File:      w.File,
		ThumbFile: w.ThumbFile,
		W:         w.W,
		H:         w.H,
		Bytes:     w.Bytes,
		SortOrder: w.SortOrder,
	}
}

func (s *Service) ListWallpapers(ctx context.Context) ([]WallpaperDTO, error) {
	rows, err := s.Q.ListWallpapers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list wallpapers: %w", err)
	}
	out := make([]WallpaperDTO, 0, len(rows))
	for _, w := range rows {
		out = append(out, toWallpaperDTO(w))
	}
	return out, nil
}

// AddWallpaperUpload 保存上传的壁纸：原图原样存，另生成 1920 宽的 JPEG 缩略图。
func (s *Service) AddWallpaperUpload(ctx context.Context, id string, data []byte) (*WallpaperDTO, error) {
	if id == "" {
		return nil, BadRequest("id is required")
	}
	if s.Wallpapers == nil || s.Thumbs == nil {
		return nil, &Error{Status: 503, Code: "wallpapers_disabled", Message: "wallpaper storage is not configured"}
	}
	if len(data) > imageproc.MaxWallpaperBytes {
		return nil, BadRequest(fmt.Sprintf("wallpaper exceeds %d bytes", imageproc.MaxWallpaperBytes))
	}

	thumb, tw, th, err := imageproc.Thumbnail(data, 1920, 82)
	if err != nil {
		return nil, BadRequest("unsupported image: " + err.Error())
	}
	if _, _, err := imageproc.Dimensions(data); err != nil {
		return nil, BadRequest("unsupported image: " + err.Error())
	}

	// 原图按真实格式存（内容寻址，重复上传同一张不会占两份）。
	// 注意用 imageproc.Format 而不是 favicon.Inspect：后者有 2MiB 图标上限，
	// 会把大图误判成"非图片"并退回错误扩展名。
	mime, err := imageproc.Format(data)
	if err != nil {
		return nil, BadRequest("unsupported image: " + err.Error())
	}
	file, err := s.Wallpapers.Save(data, mime)
	if err != nil {
		return nil, fmt.Errorf("store wallpaper: %w", err)
	}
	thumbFile, err := s.Thumbs.Save(thumb, "image/jpeg")
	if err != nil {
		return nil, fmt.Errorf("store wallpaper thumb: %w", err)
	}

	max, err := s.Q.MaxWallpaperSortOrder(ctx)
	if err != nil {
		return nil, fmt.Errorf("max wallpaper order: %w", err)
	}
	size := int64(len(data))
	ww, hh := int64(tw), int64(th)
	if err := s.Q.CreateWallpaper(ctx, dbgen.CreateWallpaperParams{
		ID:        id,
		Kind:      "upload",
		File:      &file,
		ThumbFile: &thumbFile,
		W:         &ww,
		H:         &hh,
		Bytes:     &size,
		SortOrder: asInt64(max) + 1,
	}); err != nil {
		return nil, fmt.Errorf("create wallpaper: %w", err)
	}
	created, err := s.Q.GetWallpaper(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reload wallpaper: %w", err)
	}
	dto := toWallpaperDTO(created)
	return &dto, nil
}

// AddWallpaperURL 只登记外链（前端直接 <img src>，省服务器流量），
// 想固化到本地再调 MaterializeWallpaper。
func (s *Service) AddWallpaperURL(ctx context.Context, id, rawURL string) (*WallpaperDTO, error) {
	if id == "" {
		return nil, BadRequest("id is required")
	}
	if !validHTTPURL(rawURL) {
		return nil, BadRequest("remote_url must be an absolute http(s) URL")
	}
	max, err := s.Q.MaxWallpaperSortOrder(ctx)
	if err != nil {
		return nil, fmt.Errorf("max wallpaper order: %w", err)
	}
	url := strings.TrimSpace(rawURL)
	if err := s.Q.CreateWallpaper(ctx, dbgen.CreateWallpaperParams{
		ID:        id,
		Kind:      "url",
		RemoteUrl: &url,
		SortOrder: asInt64(max) + 1,
	}); err != nil {
		return nil, fmt.Errorf("create wallpaper: %w", err)
	}
	created, err := s.Q.GetWallpaper(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reload wallpaper: %w", err)
	}
	dto := toWallpaperDTO(created)
	return &dto, nil
}

// MaterializeWallpaper 把图床 URL 下载并固化到本地（走同一套 SSRF 防护）。
func (s *Service) MaterializeWallpaper(ctx context.Context, id string) (*WallpaperDTO, error) {
	wallpaper, err := s.Q.GetWallpaper(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("wallpaper")
		}
		return nil, fmt.Errorf("get wallpaper: %w", err)
	}
	if wallpaper.RemoteUrl == nil || *wallpaper.RemoteUrl == "" {
		return nil, BadRequest("wallpaper has no remote_url to download")
	}
	if s.Wallpapers == nil || s.Thumbs == nil {
		return nil, &Error{Status: 503, Code: "wallpapers_disabled", Message: "wallpaper storage is not configured"}
	}

	data, _, err := favicon.Download(ctx, *wallpaper.RemoteUrl, imageproc.MaxWallpaperBytes, s.AllowPrivateFetch)
	if err != nil {
		return nil, BadRequest("download failed: " + err.Error())
	}
	thumb, tw, th, err := imageproc.Thumbnail(data, 1920, 82)
	if err != nil {
		return nil, BadRequest("downloaded file is not a usable image: " + err.Error())
	}
	mime, err := imageproc.Format(data)
	if err != nil {
		return nil, BadRequest("downloaded file is not a usable image: " + err.Error())
	}
	file, err := s.Wallpapers.Save(data, mime)
	if err != nil {
		return nil, fmt.Errorf("store wallpaper: %w", err)
	}
	thumbFile, err := s.Thumbs.Save(thumb, "image/jpeg")
	if err != nil {
		return nil, fmt.Errorf("store wallpaper thumb: %w", err)
	}
	size := int64(len(data))
	ww, hh := int64(tw), int64(th)
	if err := s.Q.MaterializeWallpaper(ctx, dbgen.MaterializeWallpaperParams{
		File:      &file,
		ThumbFile: &thumbFile,
		W:         &ww,
		H:         &hh,
		Bytes:     &size,
		ID:        id,
	}); err != nil {
		return nil, fmt.Errorf("materialize wallpaper: %w", err)
	}
	updated, err := s.Q.GetWallpaper(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reload wallpaper: %w", err)
	}
	dto := toWallpaperDTO(updated)
	return &dto, nil
}

type WallpaperUpdate struct {
	SortOrder *int64  `json:"sort_order"`
	RemoteURL *string `json:"remote_url"`
}

func (s *Service) UpdateWallpaper(ctx context.Context, id string, in WallpaperUpdate) (*WallpaperDTO, error) {
	current, err := s.Q.GetWallpaper(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("wallpaper")
		}
		return nil, fmt.Errorf("get wallpaper: %w", err)
	}
	sortOrder := current.SortOrder
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}
	remote := current.RemoteUrl
	if in.RemoteURL != nil {
		if *in.RemoteURL != "" && !validHTTPURL(*in.RemoteURL) {
			return nil, BadRequest("remote_url must be an absolute http(s) URL")
		}
		remote = in.RemoteURL
	}
	if err := s.Q.UpdateWallpaperMeta(ctx, dbgen.UpdateWallpaperMetaParams{
		SortOrder: sortOrder,
		RemoteUrl: remote,
		ID:        id,
	}); err != nil {
		return nil, fmt.Errorf("update wallpaper: %w", err)
	}
	updated, err := s.Q.GetWallpaper(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reload wallpaper: %w", err)
	}
	dto := toWallpaperDTO(updated)
	return &dto, nil
}

// DeleteWallpaper 删除记录，并在**没有其它记录引用同一文件**时删掉磁盘文件
// （内容寻址后可能共享；eMMC 只有十几 G，不能留垃圾）。
func (s *Service) DeleteWallpaper(ctx context.Context, id string) error {
	wallpaper, err := s.Q.GetWallpaper(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return NotFound("wallpaper")
		}
		return fmt.Errorf("get wallpaper: %w", err)
	}
	if err := s.Q.DeleteWallpaper(ctx, id); err != nil {
		return fmt.Errorf("delete wallpaper: %w", err)
	}

	// 清掉引用计数归零的文件
	if wallpaper.File != nil && s.Wallpapers != nil {
		if n, err := s.Q.CountWallpapersByFile(ctx, wallpaper.File); err == nil && n == 0 {
			if full, err := s.Wallpapers.Resolve(*wallpaper.File); err == nil {
				_ = removeFile(full)
			}
		}
	}
	if wallpaper.ThumbFile != nil && s.Thumbs != nil {
		if n, err := s.Q.CountWallpapersByThumb(ctx, wallpaper.ThumbFile); err == nil && n == 0 {
			if full, err := s.Thumbs.Resolve(*wallpaper.ThumbFile); err == nil {
				_ = removeFile(full)
			}
		}
	}
	return nil
}

// WallpaperFile 解析 /wallpapers/{orig|thumb}/<name> 的磁盘路径。
func (s *Service) WallpaperFile(kind, name string) (string, error) {
	var store *favicon.Store
	switch kind {
	case "orig":
		store = s.Wallpapers
	case "thumb":
		store = s.Thumbs
	default:
		return "", NotFound("wallpaper file")
	}
	if store == nil {
		return "", NotFound("wallpaper file")
	}
	full, err := store.Resolve(name)
	if err != nil {
		return "", NotFound("wallpaper file")
	}
	return full, nil
}

func removeFile(path string) error { return os.Remove(path) }
