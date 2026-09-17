package nav

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/url"
	"strings"

	"github.com/ffdkj/my_nav/internal/db/dbgen"
	"github.com/ffdkj/my_nav/internal/favicon"
)

// Service 是领域层入口。它持有一个 *sql.DB 以便开启事务：
// 整板提交必须是单事务，否则前端在 409/422 之后会看到半截状态。
type Service struct {
	DB *sql.DB
	Q  *dbgen.Queries

	// 图标抓取与存储。为 nil 时相关功能降级（返回 503 / 跳过抓取），
	// 这样不关心图标的单元测试可以直接 New(db)。
	Icons   *favicon.Store
	Fetcher *favicon.Fetcher
	Log     *slog.Logger
}

type Option func(*Service)

// WithIcons 注入图标存储与抓取器。
func WithIcons(store *favicon.Store, fetcher *favicon.Fetcher) Option {
	return func(s *Service) { s.Icons = store; s.Fetcher = fetcher }
}

func WithLogger(log *slog.Logger) Option {
	return func(s *Service) { s.Log = log }
}

func New(db *sql.DB, opts ...Option) *Service {
	svc := &Service{DB: db, Q: dbgen.New(db)}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// ---------- bootstrap ----------

type Bootstrap struct {
	Pages    []PageDTO         `json:"pages"`
	Settings map[string]string `json:"settings"`
	Engines  []dbgen.Engine    `json:"engines"`
}

func (s *Service) Bootstrap(ctx context.Context) (*Bootstrap, error) {
	pages, err := s.ListPages(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	engines, err := s.Q.ListEngines(ctx)
	if err != nil {
		return nil, fmt.Errorf("list engines: %w", err)
	}
	return &Bootstrap{Pages: pages, Settings: settings, Engines: engines}, nil
}

func (s *Service) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.Q.ListSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.K] = r.V
	}
	return out, nil
}

// ---------- pages ----------

func (s *Service) ListPages(ctx context.Context) ([]PageDTO, error) {
	rows, err := s.Q.ListPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	out := make([]PageDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPageDTO(r))
	}
	return out, nil
}

type CreatePageInput struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

func (s *Service) CreatePage(ctx context.Context, in CreatePageInput) (*PageDTO, error) {
	if in.ID == "" || in.Name == "" || in.Slug == "" {
		return nil, BadRequest("id, slug and name are required")
	}
	if !validSlug(in.Slug) {
		return nil, BadRequest("slug may only contain letters, digits and dashes")
	}

	pages, err := s.Q.ListPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	var next int64
	for _, p := range pages {
		if p.SortOrder >= next {
			next = p.SortOrder + 1
		}
	}

	if err := s.Q.CreatePage(ctx, dbgen.CreatePageParams{
		ID:            in.ID,
		Slug:          in.Slug,
		Name:          in.Name,
		SortOrder:     next,
		WallpaperMode: "global",
		WallpaperID:   nil,
	}); err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}

	page, err := s.Q.GetPage(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("reload page: %w", err)
	}
	dto := toPageDTO(page)
	return &dto, nil
}

type UpdatePageInput struct {
	Name          *string `json:"name"`
	Slug          *string `json:"slug"`
	WallpaperMode *string `json:"wallpaper_mode"`
	WallpaperID   *string `json:"wallpaper_id"`
	SortOrder     *int64  `json:"sort_order"`
}

func (s *Service) UpdatePage(ctx context.Context, id string, in UpdatePageInput) (*PageDTO, error) {
	page, err := s.Q.GetPage(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("page")
		}
		return nil, fmt.Errorf("get page: %w", err)
	}

	if in.Name != nil {
		page.Name = *in.Name
	}
	if in.Slug != nil {
		if !validSlug(*in.Slug) {
			return nil, BadRequest("slug may only contain letters, digits and dashes")
		}
		page.Slug = *in.Slug
	}
	if in.WallpaperMode != nil {
		if *in.WallpaperMode != "global" && *in.WallpaperMode != "custom" {
			return nil, BadRequest("wallpaper_mode must be global or custom")
		}
		page.WallpaperMode = *in.WallpaperMode
	}
	if in.WallpaperID != nil {
		if *in.WallpaperID == "" {
			page.WallpaperID = nil
		} else {
			page.WallpaperID = in.WallpaperID
		}
	}

	if err := s.Q.UpdatePage(ctx, dbgen.UpdatePageParams{
		Name:          page.Name,
		Slug:          page.Slug,
		WallpaperMode: page.WallpaperMode,
		WallpaperID:   page.WallpaperID,
		ID:            id,
	}); err != nil {
		return nil, fmt.Errorf("update page: %w", err)
	}

	if in.SortOrder != nil {
		if err := s.Q.UpdatePageSortOrder(ctx, dbgen.UpdatePageSortOrderParams{
			SortOrder: *in.SortOrder,
			ID:        id,
		}); err != nil {
			return nil, fmt.Errorf("update page order: %w", err)
		}
	}

	updated, err := s.Q.GetPage(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reload page: %w", err)
	}
	dto := toPageDTO(updated)
	return &dto, nil
}

func (s *Service) DeletePage(ctx context.Context, id string) error {
	if _, err := s.Q.GetPage(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return NotFound("page")
		}
		return fmt.Errorf("get page: %w", err)
	}
	count, err := s.Q.CountPages(ctx)
	if err != nil {
		return fmt.Errorf("count pages: %w", err)
	}
	if count <= 1 {
		return &Error{
			Status:  422,
			Code:    "last_page",
			Message: "the last remaining page cannot be deleted",
		}
	}
	if err := s.Q.DeletePage(ctx, id); err != nil {
		return fmt.Errorf("delete page: %w", err)
	}
	return nil
}

// ---------- board ----------

func (s *Service) GetBoard(ctx context.Context, pageID string) (*Board, error) {
	page, err := s.Q.GetPage(ctx, pageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("page")
		}
		return nil, fmt.Errorf("get page: %w", err)
	}

	placements, err := s.Q.ListPlacementsForPage(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list placements: %w", err)
	}
	links, err := s.Q.ListLinksForPage(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	folders, err := s.Q.ListFoldersForPage(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}

	return assembleBoard(page, placements, links, folders), nil
}

// PutBoard 在单事务内整板替换。revision 不匹配返回 409，校验失败返回 422。
func (s *Service) PutBoard(ctx context.Context, pageID string, payload *BoardPayload) (*Board, error) {
	page, err := s.Q.GetPage(ctx, pageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("page")
		}
		return nil, fmt.Errorf("get page: %w", err)
	}
	if payload.Revision != page.Revision {
		return nil, RevisionMismatch(page.Revision, payload.Revision)
	}

	existingLinks, err := s.Q.ListLinksForPage(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	existingFolders, err := s.Q.ListFoldersForPage(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}

	knownLinks := make(map[string]struct{}, len(existingLinks))
	for _, l := range existingLinks {
		knownLinks[l.ID] = struct{}{}
	}
	knownFolders := make(map[string]struct{}, len(existingFolders))
	for _, f := range existingFolders {
		knownFolders[f.ID] = struct{}{}
	}

	// 纯函数校验：结构与网格都在这里判定，失败时不碰数据库。
	if err := ValidateBoard(payload, knownLinks, knownFolders); err != nil {
		return nil, err
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := dbgen.New(tx)

	for _, l := range payload.NewLinks {
		openNewTab := int64(1)
		if l.OpenNewTab != nil && !*l.OpenNewTab {
			openNewTab = 0
		}
		url := strings.TrimSpace(l.URL)
		title := strings.TrimSpace(l.Title)
		if title == "" {
			title = hostOf(url)
		}
		if err := q.CreateLink(ctx, dbgen.CreateLinkParams{
			ID:           l.ID,
			Title:        title,
			Url:          url,
			OpenNewTab:   openNewTab,
			IconSource:   "auto",
			IconStatus:   "pending",
			MonoText:     nil,
			MonoColor:    monogramColor(url),
			MonoFontSize: 30,
		}); err != nil {
			return nil, fmt.Errorf("create link %s: %w", l.ID, err)
		}
	}

	// new_folders 的语义是 upsert：不存在则建，存在则更新名字/尺寸。
	// 前端因此可以在"把 1 格夹放大成 2x2"时复用同一个字段，不需要额外的更新端点。
	for _, f := range payload.NewFolders {
		size := f.Size
		if size == 0 {
			size = 1
		}
		if size != 1 && size != 2 {
			return nil, BadRequest("folder size must be 1 or 2")
		}
		if _, exists := knownFolders[f.ID]; exists {
			if err := q.UpdateFolder(ctx, dbgen.UpdateFolderParams{Name: f.Name, Size: size, ID: f.ID}); err != nil {
				return nil, fmt.Errorf("update folder %s: %w", f.ID, err)
			}
			continue
		}
		if err := q.CreateFolder(ctx, dbgen.CreateFolderParams{ID: f.ID, Name: f.Name, Size: size}); err != nil {
			return nil, fmt.Errorf("create folder %s: %w", f.ID, err)
		}
	}

	// 整板替换：先清掉该页所有 placement，再按负载重建。
	if err := q.DeletePlacementsForPage(ctx, pageID); err != nil {
		return nil, fmt.Errorf("clear placements: %w", err)
	}

	for _, it := range payload.Items {
		switch it.Kind {
		case "link":
			linkID := it.LinkID
			if err := q.CreatePlacement(ctx, dbgen.CreatePlacementParams{
				ID:       it.ID,
				PageID:   pageID,
				LinkID:   &linkID,
				Col:      it.Col,
				Row:      it.Row,
				FolderID: nil,
				InFolder: nil,
			}); err != nil {
				return nil, fmt.Errorf("create placement %s: %w", it.ID, err)
			}
		case "folder":
			folderID := it.FolderID
			size := it.Size
			if size == 0 {
				size = 1
			}
			if err := q.CreatePlacement(ctx, dbgen.CreatePlacementParams{
				ID:        it.ID,
				PageID:    pageID,
				FolderID:  &folderID,
				LinkID:    nil,
				InFolder:  nil,
				Col:       it.Col,
				Row:       it.Row,
				SortOrder: 0,
			}); err != nil {
				return nil, fmt.Errorf("create folder placement %s: %w", it.ID, err)
			}
			for i, c := range it.Children {
				linkID := c.LinkID
				if err := q.CreatePlacement(ctx, dbgen.CreatePlacementParams{
					ID:        c.ID,
					PageID:    pageID,
					LinkID:    &linkID,
					FolderID:  nil,
					InFolder:  &folderID,
					Col:       0,
					Row:       0,
					SortOrder: int64(i),
				}); err != nil {
					return nil, fmt.Errorf("create folder child %s: %w", c.ID, err)
				}
			}
		}
	}

	for _, id := range payload.DeletedLinkIDs {
		if err := q.DeleteLink(ctx, id); err != nil {
			return nil, fmt.Errorf("delete link %s: %w", id, err)
		}
	}
	for _, id := range payload.DeletedFolderIDs {
		if err := q.DeleteFolder(ctx, id); err != nil {
			return nil, fmt.Errorf("delete folder %s: %w", id, err)
		}
	}

	// 不变量：空文件夹自动删除（例如把最后一个图标拖出去之后）。
	empties, err := q.ListEmptyFoldersForPage(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list empty folders: %w", err)
	}
	for _, id := range empties {
		if err := q.DeleteFolder(ctx, id); err != nil {
			return nil, fmt.Errorf("delete empty folder %s: %w", id, err)
		}
	}

	if _, err := q.BumpPageRevision(ctx, pageID); err != nil {
		return nil, fmt.Errorf("bump revision: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit board: %w", err)
	}

	// 事务提交之后再抓图标：抓取是网络 I/O，放在事务里会一直握着 SQLite 写锁
	s.refreshIconsForPage(ctx, pageID)

	return s.GetBoard(ctx, pageID)
}

// ---------- 装配与工具 ----------

func assembleBoard(page dbgen.Page, placements []dbgen.Placement, links []dbgen.Link, folders []dbgen.Folder) *Board {
	board := &Board{
		Page:     toPageDTO(page),
		Revision: page.Revision,
		Items:    make([]ItemDTO, 0, len(placements)),
		Links:    make([]LinkDTO, 0, len(links)),
		Folders:  make([]FolderDTO, 0, len(folders)),
	}

	for _, l := range links {
		board.Links = append(board.Links, toLinkDTO(l))
	}
	for _, f := range folders {
		board.Folders = append(board.Folders, FolderDTO{ID: f.ID, Name: f.Name, Size: f.Size})
	}

	childrenByFolder := make(map[string][]ChildDTO)
	for _, p := range placements {
		if p.InFolder == nil || p.LinkID == nil {
			continue
		}
		childrenByFolder[*p.InFolder] = append(childrenByFolder[*p.InFolder], ChildDTO{
			ID:        p.ID,
			LinkID:    *p.LinkID,
			SortOrder: p.SortOrder,
		})
	}

	for _, p := range placements {
		if p.InFolder != nil {
			continue // 夹内项由上面的 children 承载
		}
		item := ItemDTO{ID: p.ID, Col: p.Col, Row: p.Row}
		switch {
		case p.LinkID != nil:
			item.Kind = "link"
			item.LinkID = *p.LinkID
		case p.FolderID != nil:
			item.Kind = "folder"
			item.FolderID = *p.FolderID
			item.Size = 1
			if f, ok := findFolder(folders, *p.FolderID); ok {
				item.Size = f.Size
			}
			item.Children = childrenByFolder[*p.FolderID]
		}
		board.Items = append(board.Items, item)
	}

	return board
}

func findFolder(folders []dbgen.Folder, id string) (dbgen.Folder, bool) {
	for _, f := range folders {
		if f.ID == id {
			return f, true
		}
	}
	return dbgen.Folder{}, false
}

func toPageDTO(p dbgen.Page) PageDTO {
	return PageDTO{
		ID:            p.ID,
		Slug:          p.Slug,
		Name:          p.Name,
		SortOrder:     p.SortOrder,
		Revision:      p.Revision,
		WallpaperMode: p.WallpaperMode,
		WallpaperID:   p.WallpaperID,
	}
}

func toLinkDTO(l dbgen.Link) LinkDTO {
	return LinkDTO{
		ID:           l.ID,
		Title:        l.Title,
		URL:          l.Url,
		OpenNewTab:   l.OpenNewTab != 0,
		IconSource:   l.IconSource,
		IconPath:     l.IconPath,
		IconStatus:   l.IconStatus,
		MonoText:     l.MonoText,
		MonoColor:    l.MonoColor,
		MonoFontSize: l.MonoFontSize,
	}
}

// monogramColor 按域名稳定地挑一个兜底色（M5 抓到真图标后就不再使用）。
func monogramColor(rawURL string) string {
	host := hostOf(rawURL)
	h := fnv.New32a()
	_, _ = h.Write([]byte(host))
	palette := [...]string{
		"#3B82F6", "#8B5CF6", "#EC4899", "#F97316", "#EAB308",
		"#22C55E", "#14B8A6", "#06B6D4", "#6366F1", "#EF4444",
	}
	return palette[int(h.Sum32())%len(palette)]
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return strings.TrimPrefix(u.Host, "www.")
}

func validSlug(slug string) bool {
	if slug == "" || len(slug) > 64 {
		return false
	}
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}
