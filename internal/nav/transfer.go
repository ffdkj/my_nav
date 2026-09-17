package nav

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ffdkj/my_nav/internal/db/dbgen"
	"github.com/ffdkj/my_nav/internal/favicon"
)

// ExportVersion 是导出文档的版本号。导入时严格校验，避免把旧格式当新格式读。
const ExportVersion = 1

// preImportBackupName 是导入前自动快照的固定文件名——**服务器上永远只有这一份**。
const preImportBackupName = "pre-import.json"

// ExportPage 把页面与它的布局放在一起，这样导出的 JSON 是"人可读、可手改"的：
// 想删掉某个图标，直接在 items 里删一行即可。
type ExportPage struct {
	PageDTO
	Items []ItemDTO `json:"items"`
}

type ExportDocument struct {
	Version    int               `json:"version"`
	ExportedAt string            `json:"exported_at"`
	Settings   map[string]string `json:"settings"`
	Engines    []EngineDTO       `json:"engines"`
	Wallpapers []WallpaperDTO    `json:"wallpapers"`
	Pages      []ExportPage      `json:"pages"`
	Links      []LinkDTO         `json:"links"`
	Folders    []FolderDTO       `json:"folders"`
}

// ---------- 导出 ----------

func (s *Service) Export(ctx context.Context) (*ExportDocument, error) {
	doc := &ExportDocument{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
	}

	settings, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	doc.Settings = settings

	if doc.Engines, err = s.ListEngines(ctx); err != nil {
		return nil, err
	}
	if doc.Wallpapers, err = s.ListWallpapers(ctx); err != nil {
		return nil, err
	}

	pages, err := s.Q.ListPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	links, err := s.Q.ListLinks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	folders, err := s.Q.ListFolders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}

	doc.Links = make([]LinkDTO, 0, len(links))
	for _, l := range links {
		doc.Links = append(doc.Links, toLinkDTO(l))
	}
	doc.Folders = make([]FolderDTO, 0, len(folders))
	for _, f := range folders {
		doc.Folders = append(doc.Folders, FolderDTO{ID: f.ID, Name: f.Name, Size: f.Size})
	}

	for _, p := range pages {
		dto := toPageDTO(p)
		dto.Revision = 0 // 导入后 revision 从 0 重新开始
		items, err := s.pageItems(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		doc.Pages = append(doc.Pages, ExportPage{PageDTO: dto, Items: items})
	}
	return doc, nil
}

// pageItems 把某页的 placements 组装成与 board PUT 相同的形状。
func (s *Service) pageItems(ctx context.Context, pageID string) ([]ItemDTO, error) {
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
	page, err := s.Q.GetPage(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("get page: %w", err)
	}
	return assembleBoard(page, placements, links, folders).Items, nil
}

// ExportJSON 返回格式化后的 JSON 字节（带缩进，方便人读与 diff）。
func (s *Service) ExportJSON(ctx context.Context) ([]byte, error) {
	doc, err := s.Export(ctx)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(doc, "", "  ")
}

// ExportZip 打包 data.json 与所有被引用的图标/壁纸二进制。
// 目录结构与 /data 一致，导入时可以原样写回。
func (s *Service) ExportZip(ctx context.Context) ([]byte, error) {
	doc, err := s.Export(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	if err := writeZipEntry(archive, "data.json", raw); err != nil {
		return nil, err
	}

	// 图标：只打包被链接引用的（少打包一堆无用文件）
	seen := map[string]struct{}{}
	for _, link := range doc.Links {
		if link.IconPath == nil || *link.IconPath == "" {
			continue
		}
		rel := *link.IconPath
		if _, dup := seen["icons/"+rel]; dup {
			continue
		}
		seen["icons/"+rel] = struct{}{}
		if s.Icons == nil {
			continue
		}
		full, err := s.Icons.Resolve(rel)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			continue // 记录里有、磁盘上没了：跳过而不是让导出失败
		}
		if err := writeZipEntry(archive, "icons/"+filepath.ToSlash(rel), data); err != nil {
			return nil, err
		}
	}

	// 壁纸：原图 + 缩略图
	for _, w := range doc.Wallpapers {
		if w.File != nil {
			if err := addStoreFile(archive, s.Wallpapers, "wallpapers/orig/", *w.File); err != nil {
				return nil, err
			}
		}
		if w.ThumbFile != nil {
			if err := addStoreFile(archive, s.Thumbs, "wallpapers/thumb/", *w.ThumbFile); err != nil {
				return nil, err
			}
		}
	}

	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeZipEntry(archive *zip.Writer, name string, data []byte) error {
	w, err := archive.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func addStoreFile(archive *zip.Writer, store interface{ Resolve(string) (string, error) }, prefix, rel string) error {
	if store == nil {
		return nil
	}
	full, err := store.Resolve(rel)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil // 文件缺失不该让导出整体失败
	}
	return writeZipEntry(archive, prefix+filepath.ToSlash(rel), data)
}

// ---------- 导入前的自动快照 ----------

type BackupInfo struct {
	Exists bool   `json:"exists"`
	At     string `json:"at,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
}

// WritePreImportBackup 覆盖式写入唯一的导入前快照。
func (s *Service) WritePreImportBackup(ctx context.Context) error {
	if s.BackupDir == "" {
		return nil
	}
	raw, err := s.ExportJSON(ctx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.BackupDir, 0o750); err != nil {
		return fmt.Errorf("mkdir backup: %w", err)
	}
	target := filepath.Join(s.BackupDir, preImportBackupName)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o640); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	// 先写临时文件再 rename：快照要么是完整的旧状态，要么不存在
	return os.Rename(tmp, target)
}

func (s *Service) BackupInfo() BackupInfo {
	if s.BackupDir == "" {
		return BackupInfo{}
	}
	info, err := os.Stat(filepath.Join(s.BackupDir, preImportBackupName))
	if err != nil {
		return BackupInfo{}
	}
	return BackupInfo{
		Exists: true,
		At:     info.ModTime().UTC().Format(time.RFC3339),
		Bytes:  info.Size(),
	}
}

func (s *Service) BackupPath() string {
	return filepath.Join(s.BackupDir, preImportBackupName)
}

// ---------- 导入 ----------

// ParseImport 解析上传内容：zip 则拆出 data.json 与资源，否则按纯 JSON 处理。
func ParseImport(filename string, data []byte) (*ExportDocument, map[string][]byte, error) {
	isZip := strings.HasSuffix(strings.ToLower(filename), ".zip") ||
		(len(data) > 2 && data[0] == 'P' && data[1] == 'K')
	if !isZip {
		var doc ExportDocument
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, nil, BadRequest("invalid JSON: " + err.Error())
		}
		return &doc, nil, nil
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, BadRequest("invalid zip: " + err.Error())
	}
	assets := map[string][]byte{}
	var doc *ExportDocument
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, nil, BadRequest("cannot read zip entry " + f.Name)
		}
		content, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		_ = rc.Close()
		if err != nil {
			return nil, nil, BadRequest("cannot read zip entry " + f.Name)
		}

		clean := path.Clean("/" + f.Name)
		clean = strings.TrimPrefix(clean, "/")
		if clean == "data.json" {
			var parsed ExportDocument
			if err := json.Unmarshal(content, &parsed); err != nil {
				return nil, nil, BadRequest("data.json is not valid JSON: " + err.Error())
			}
			doc = &parsed
			continue
		}
		// 只接受三类资源，且必须留在各自目录里（防止 zip slip）
		switch {
		case strings.HasPrefix(clean, "icons/"),
			strings.HasPrefix(clean, "wallpapers/orig/"),
			strings.HasPrefix(clean, "wallpapers/thumb/"):
			if strings.Contains(clean, "..") {
				continue
			}
			assets[clean] = content
		}
	}
	if doc == nil {
		return nil, nil, BadRequest("zip does not contain data.json")
	}
	return doc, assets, nil
}

// Import 全量覆盖导入。顺序刻意如此：
//  1. 先校验（坏数据要在破坏现状之前被拒）
//  2. 再写导入前快照（唯一一份）
//  3. 落资源文件（内容寻址，纯增量，失败也不影响现有数据）
//  4. 最后单事务换库
func (s *Service) Import(ctx context.Context, doc *ExportDocument, assets map[string][]byte) error {
	if doc.Version != ExportVersion {
		return BadRequest(fmt.Sprintf("unsupported export version %d (expected %d)", doc.Version, ExportVersion))
	}
	if len(doc.Pages) == 0 {
		return BadRequest("export contains no pages")
	}

	knownLinks := map[string]struct{}{}
	for _, l := range doc.Links {
		if l.ID == "" {
			return BadRequest("a link is missing its id")
		}
		knownLinks[l.ID] = struct{}{}
	}
	knownFolders := map[string]struct{}{}
	for _, f := range doc.Folders {
		if f.ID == "" {
			return BadRequest("a folder is missing its id")
		}
		knownFolders[f.ID] = struct{}{}
	}

	seenSlugs := map[string]struct{}{}
	for _, p := range doc.Pages {
		if p.ID == "" || p.Slug == "" {
			return BadRequest("a page is missing id or slug")
		}
		if _, dup := seenSlugs[p.Slug]; dup {
			return BadRequest("duplicate page slug: " + p.Slug)
		}
		seenSlugs[p.Slug] = struct{}{}
		// 复用与整板提交相同的校验：网格/容量/嵌套/引用一次判清
		if err := ValidateBoard(&BoardPayload{Items: p.Items}, knownLinks, knownFolders); err != nil {
			return Invalid("page " + p.Slug + ": " + AsError(err).Message)
		}
	}

	for _, e := range doc.Engines {
		if in := (EngineInput{
			Name: e.Name, URLTpl: e.URLTpl, IconText: e.IconText, IconColor: e.IconColor,
		}); validateEngine(in) != nil {
			return Invalid("engine " + e.Name + ": " + validateEngine(in).Message)
		}
	}

	if err := s.WritePreImportBackup(ctx); err != nil {
		return fmt.Errorf("pre-import backup failed: %w", err)
	}

	// 资源先落盘（纯增量）
	if err := s.restoreAssets(assets); err != nil {
		return err
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := dbgen.New(tx)

	for _, table := range []string{"placements", "folders", "links", "pages", "engines", "wallpapers", "settings"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}

	// ⚠️ 顺序很关键（外键）：pages → links → folders → placements。
	// 先插 placements 会撞上 placements.link_id 的外键约束。
	for _, p := range doc.Pages {
		if err := q.CreatePage(ctx, dbgen.CreatePageParams{
			ID:            p.ID,
			Slug:          p.Slug,
			Name:          p.Name,
			SortOrder:     p.SortOrder,
			WallpaperMode: orDefault(p.WallpaperMode, "global"),
			WallpaperID:   p.WallpaperID,
		}); err != nil {
			return fmt.Errorf("insert page %s: %w", p.Slug, err)
		}
		if _, err := q.BumpPageRevision(ctx, p.ID); err != nil {
			return fmt.Errorf("init page revision: %w", err)
		}
	}

	for _, l := range doc.Links {
		if err := q.CreateLink(ctx, dbgen.CreateLinkParams{
			ID:            l.ID,
			Title:         l.Title,
			Url:           l.URL,
			OpenNewTab:    boolToInt(l.OpenNewTab),
			IconSource:    orDefault(l.IconSource, "auto"),
			IconPath:      l.IconPath,
			IconStatus:    s.iconStatusAfterImport(l),
			MonoText:      l.MonoText,
			MonoColor:     orDefault(l.MonoColor, "#3B82F6"),
			MonoFontSize:  orDefaultInt(l.MonoFontSize, 30),
			IconCheckedAt: nil,
		}); err != nil {
			return fmt.Errorf("insert link %s: %w", l.ID, err)
		}
	}

	for _, f := range doc.Folders {
		size := f.Size
		if size != 2 {
			size = 1
		}
		if err := q.CreateFolder(ctx, dbgen.CreateFolderParams{ID: f.ID, Name: f.Name, Size: size}); err != nil {
			return fmt.Errorf("insert folder %s: %w", f.ID, err)
		}
	}

	// 链接与文件夹都就位后再插布局，外键才成立
	for _, p := range doc.Pages {
		if err := s.insertItems(ctx, q, p.ID, p.Items, knownLinks, knownFolders); err != nil {
			return err
		}
	}

	for _, e := range doc.Engines {
		if err := q.CreateEngine(ctx, dbgen.CreateEngineParams{
			ID:        e.ID,
			Name:      e.Name,
			UrlTpl:    e.URLTpl,
			IconText:  e.IconText,
			IconColor: e.IconColor,
			SortOrder: e.SortOrder,
		}); err != nil {
			return fmt.Errorf("insert engine %s: %w", e.ID, err)
		}
	}
	// 导入文档里没有内置引擎时（例如手工裁剪过），补回缺失的那几个，
	// 否则搜索框可能没有任何可选引擎。
	if err := s.ensureBuiltinEngines(ctx, q, doc.Engines); err != nil {
		return err
	}

	for _, w := range doc.Wallpapers {
		if err := q.CreateWallpaper(ctx, dbgen.CreateWallpaperParams{
			ID:        w.ID,
			Kind:      orDefault(w.Kind, "url"),
			RemoteUrl: w.RemoteURL,
			File:      w.File,
			ThumbFile: w.ThumbFile,
			W:         w.W,
			H:         w.H,
			Bytes:     w.Bytes,
			SortOrder: w.SortOrder,
		}); err != nil {
			return fmt.Errorf("insert wallpaper %s: %w", w.ID, err)
		}
	}

	for k, v := range doc.Settings {
		if _, ok := writableSettings[k]; !ok {
			continue // 忽略未知键，避免导入把库塞脏
		}
		if err := q.UpsertSetting(ctx, dbgen.UpsertSettingParams{K: k, V: v}); err != nil {
			return fmt.Errorf("insert setting %s: %w", k, err)
		}
	}

	return tx.Commit()
}

func (s *Service) insertItems(
	ctx context.Context, q *dbgen.Queries, pageID string,
	items []ItemDTO, knownLinks, knownFolders map[string]struct{},
) error {
	for _, it := range items {
		switch it.Kind {
		case "link":
			linkID := it.LinkID
			if err := q.CreatePlacement(ctx, dbgen.CreatePlacementParams{
				ID: it.ID, PageID: pageID, LinkID: &linkID, Col: it.Col, Row: it.Row,
			}); err != nil {
				return fmt.Errorf("insert placement %s: %w", it.ID, err)
			}
		case "folder":
			folderID := it.FolderID
			if err := q.CreatePlacement(ctx, dbgen.CreatePlacementParams{
				ID: it.ID, PageID: pageID, FolderID: &folderID, Col: it.Col, Row: it.Row,
			}); err != nil {
				return fmt.Errorf("insert folder placement %s: %w", it.ID, err)
			}
			for i, c := range it.Children {
				linkID := c.LinkID
				if err := q.CreatePlacement(ctx, dbgen.CreatePlacementParams{
					ID: c.ID, PageID: pageID, LinkID: &linkID, InFolder: &folderID, SortOrder: int64(i),
				}); err != nil {
					return fmt.Errorf("insert folder child %s: %w", c.ID, err)
				}
			}
		}
	}
	return nil
}

// restoreAssets 把 zip 里的图标/壁纸写回内容寻址目录。纯增量，不会破坏已有文件。
func (s *Service) restoreAssets(assets map[string][]byte) error {
	if len(assets) == 0 {
		return nil
	}
	// 目标目录 = zip 里的前缀 → 内容寻址目录
	type target struct {
		prefix string
		dir    string
	}
	targets := []target{
		{"icons/", dirOf(s.Icons)},
		{"wallpapers/orig/", dirOf(s.Wallpapers)},
		{"wallpapers/thumb/", dirOf(s.Thumbs)},
	}

	names := make([]string, 0, len(assets))
	for name := range assets {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		var dest string
		for _, t := range targets {
			if t.dir != "" && strings.HasPrefix(name, t.prefix) {
				dest = t.dir
				name = strings.TrimPrefix(name, t.prefix)
				break
			}
		}
		if dest == "" {
			continue
		}
		rel := filepath.FromSlash(name)
		if strings.Contains(rel, "..") {
			continue // zip slip
		}
		full := filepath.Join(dest, rel)
		if !strings.HasPrefix(filepath.Clean(full), filepath.Clean(dest)) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			return fmt.Errorf("mkdir for %s: %w", full, err)
		}
		if _, err := os.Stat(full); err == nil {
			continue // 同内容已存在（内容寻址 ⇒ 相同路径即相同内容）
		}
		if err := os.WriteFile(full, assets[strings.TrimPrefix(name, "")], 0o640); err != nil {
			return fmt.Errorf("restore %s: %w", full, err)
		}
	}
	return nil
}

func dirOf(store *favicon.Store) string {
	if store == nil {
		return ""
	}
	return store.Dir
}

// iconStatusAfterImport：只有图标文件真的在磁盘上才保留 ok，
// 否则降级为 miss（前端显示纯色兜底），用户可以手动"重新抓取"。
func (s *Service) iconStatusAfterImport(l LinkDTO) string {
	if l.IconPath == nil || *l.IconPath == "" || s.Icons == nil {
		return "miss"
	}
	full, err := s.Icons.Resolve(*l.IconPath)
	if err != nil {
		return "miss"
	}
	if _, err := os.Stat(full); err != nil {
		return "miss"
	}
	return "ok"
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func orDefaultInt(v int64, fallback int64) int64 {
	if v == 0 {
		return fallback
	}
	return v
}

// builtinEngines 与 001_init.sql 里的种子保持一致：
// 导入的文档若被手工裁剪掉内置引擎，这里补回来，免得搜索框没有可选引擎。
var builtinEngines = []EngineDTO{
	{ID: "eng-google", Name: "Google", URLTpl: "https://www.google.com/search?q={query}", IconText: "G", IconColor: "#4285F4", SortOrder: 10, IsBuiltin: true},
	{ID: "eng-bing", Name: "Bing", URLTpl: "https://www.bing.com/search?q={query}", IconText: "b", IconColor: "#0F7B6C", SortOrder: 20, IsBuiltin: true},
	{ID: "eng-baidu", Name: "Baidu", URLTpl: "https://www.baidu.com/s?wd={query}", IconText: "B", IconColor: "#2932E1", SortOrder: 30, IsBuiltin: true},
	{ID: "eng-ddgo", Name: "DuckDuckGo", URLTpl: "https://duckduckgo.com/?q={query}", IconText: "D", IconColor: "#DE5833", SortOrder: 40, IsBuiltin: true},
	{ID: "eng-sogou", Name: "Sogou", URLTpl: "https://www.sogou.com/web?query={query}", IconText: "S", IconColor: "#FD6C1C", SortOrder: 50, IsBuiltin: true},
	{ID: "eng-youdao", Name: "Youdao", URLTpl: "https://www.youdao.com/result?word={query}", IconText: "Y", IconColor: "#D93B3B", SortOrder: 60, IsBuiltin: true},
}

func (s *Service) ensureBuiltinEngines(ctx context.Context, q *dbgen.Queries, imported []EngineDTO) error {
	have := make(map[string]struct{}, len(imported))
	for _, e := range imported {
		have[e.ID] = struct{}{}
	}
	for _, e := range builtinEngines {
		if _, ok := have[e.ID]; ok {
			continue
		}
		if err := q.CreateEngine(ctx, dbgen.CreateEngineParams{
			ID: e.ID, Name: e.Name, UrlTpl: e.URLTpl,
			IconText: e.IconText, IconColor: e.IconColor, SortOrder: e.SortOrder,
		}); err != nil {
			return fmt.Errorf("restore builtin engine %s: %w", e.ID, err)
		}
	}
	return nil
}
