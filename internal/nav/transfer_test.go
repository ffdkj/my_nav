package nav

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ffdkj/my_nav/internal/db"
	"github.com/ffdkj/my_nav/internal/db/dbgen"
	"github.com/ffdkj/my_nav/internal/favicon"
	"github.com/ffdkj/my_nav/internal/migrate"
)

// seedRichState 造一份"什么都有"的状态：两页、文件夹（含子项）、引擎、壁纸、非默认设置。
func seedRichState(t *testing.T, svc *Service, ctx context.Context) {
	t.Helper()

	if _, err := svc.PutBoard(ctx, testPage, &BoardPayload{
		Revision: 0,
		NewLinks: []LinkInput{
			{ID: "l1", URL: "https://github.com", Title: "GitHub"},
			{ID: "l2", URL: "https://gitee.com", Title: "Gitee"},
			{ID: "l3", URL: "https://go.dev", Title: "Go"},
		},
		NewFolders: []FolderInput{{ID: "f1", Size: 2}},
		Items: []ItemDTO{
			linkItem("i1", "l1", 0, 0),
			{
				ID: "i2", Kind: "folder", FolderID: "f1", Size: 2, Col: 2, Row: 0,
				Children: []ChildDTO{
					{ID: "c1", LinkID: "l2", SortOrder: 0},
					{ID: "c2", LinkID: "l3", SortOrder: 1},
				},
			},
		},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}
	if _, err := svc.CreatePage(ctx, CreatePageInput{ID: "page-work", Slug: "work", Name: "Work"}); err != nil {
		t.Fatalf("create page: %v", err)
	}
	if _, err := svc.PutBoard(ctx, "page-work", &BoardPayload{
		Revision: 0,
		NewLinks: []LinkInput{{ID: "l9", URL: "https://x.dev", Title: "X"}},
		Items:    []ItemDTO{linkItem("i9", "l9", 5, 3)},
	}); err != nil {
		t.Fatalf("seed work board: %v", err)
	}
	if _, err := svc.CreateEngine(ctx, EngineInput{
		ID: "eng-mine", Name: "Mine", URLTpl: "https://m.dev/?q={query}",
		IconText: "M", IconColor: "#3B82F6",
	}); err != nil {
		t.Fatalf("create engine: %v", err)
	}
	if _, err := svc.AddWallpaperURL(ctx, "wp1", "https://images.example.com/a.jpg"); err != nil {
		t.Fatalf("add wallpaper: %v", err)
	}
	if err := svc.UpdateSettings(ctx, map[string]string{"merge_dwell_ms": "650", "theme": "light"}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	svc, ctx, _ := newMediaService(t)
	seedRichState(t, svc, ctx)

	before, err := svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// 自导入：用同一份文档全量覆盖自己
	if err := svc.Import(ctx, before, nil); err != nil {
		t.Fatalf("Import: %v", err)
	}

	after, err := svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export after import: %v", err)
	}

	if len(after.Pages) != len(before.Pages) {
		t.Fatalf("pages = %d, want %d", len(after.Pages), len(before.Pages))
	}
	if len(after.Links) != len(before.Links) {
		t.Errorf("links = %d, want %d", len(after.Links), len(before.Links))
	}
	if len(after.Folders) != len(before.Folders) {
		t.Errorf("folders = %d, want %d", len(after.Folders), len(before.Folders))
	}
	if len(after.Engines) != len(before.Engines) {
		t.Errorf("engines = %d, want %d", len(after.Engines), len(before.Engines))
	}
	if len(after.Wallpapers) != len(before.Wallpapers) {
		t.Errorf("wallpapers = %d, want %d", len(after.Wallpapers), len(before.Wallpapers))
	}
	if after.Settings["merge_dwell_ms"] != "650" || after.Settings["theme"] != "light" {
		t.Errorf("设置未还原：%v", after.Settings)
	}

	// 布局逐项比对（含 2x2 文件夹与夹内子项）
	bySlug := map[string]ExportPage{}
	for _, p := range after.Pages {
		bySlug[p.Slug] = p
	}
	home := bySlug["home"]
	if len(home.Items) != 2 {
		t.Fatalf("home items = %d, want 2", len(home.Items))
	}
	var folder *ItemDTO
	for i := range home.Items {
		if home.Items[i].Kind == "folder" {
			folder = &home.Items[i]
		}
	}
	if folder == nil || folder.Size != 2 || len(folder.Children) != 2 || folder.Col != 2 {
		t.Errorf("文件夹未正确还原：%+v", folder)
	}
	work := bySlug["work"]
	if len(work.Items) != 1 || work.Items[0].Col != 5 || work.Items[0].Row != 3 {
		t.Errorf("第二页布局未还原：%+v", work.Items)
	}
}

func TestImportRejectsInvalidBeforeTouchingData(t *testing.T) {
	svc, ctx, _ := newMediaService(t)
	seedRichState(t, svc, ctx)

	bad, err := svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	// 制造重叠：把首页两项放到同一格
	bad.Pages[0].Items[1].Col = bad.Pages[0].Items[0].Col
	bad.Pages[0].Items[1].Row = bad.Pages[0].Items[0].Row

	err = svc.Import(ctx, bad, nil)
	apiErr := codeOf(t, err)
	if apiErr.Status != 422 {
		t.Fatalf("status = %d, want 422", apiErr.Status)
	}

	// 现有数据必须原封不动
	after, err := svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export after failed import: %v", err)
	}
	if len(after.Pages) != 2 || len(after.Links) != 4 {
		t.Errorf("导入被拒后数据被改动了：pages=%d links=%d", len(after.Pages), len(after.Links))
	}
	// 校验失败时**不该**写快照（还没有破坏现状，快照没有意义）
	if _, err := os.Stat(filepath.Join(svc.BackupDir, preImportBackupName)); !os.IsNotExist(err) {
		t.Errorf("校验失败时不应写备份快照")
	}
}

func TestImportKeepsExactlyOnePreImportBackup(t *testing.T) {
	svc, ctx, _ := newMediaService(t)
	seedRichState(t, svc, ctx)

	first, err := svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if err := svc.Import(ctx, first, nil); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// 改点东西，再导入一次：快照应被覆盖，且始终只有一份
	if err := svc.UpdateSettings(ctx, map[string]string{"merge_dwell_ms": "123"}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	if err := svc.Import(ctx, first, nil); err != nil {
		t.Fatalf("second import: %v", err)
	}

	entries, err := os.ReadDir(svc.BackupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != preImportBackupName {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("备份目录 = %v，期望只有 %s", names, preImportBackupName)
	}

	// 快照内容应是"第二次导入之前"的状态（merge_dwell_ms=123）
	raw, err := os.ReadFile(filepath.Join(svc.BackupDir, preImportBackupName))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	var doc ExportDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("backup is not valid JSON: %v", err)
	}
	if doc.Settings["merge_dwell_ms"] != "123" {
		t.Errorf("快照不是导入前的状态：merge_dwell_ms=%s", doc.Settings["merge_dwell_ms"])
	}

	info := svc.BackupInfo()
	if !info.Exists || info.Bytes == 0 || info.At == "" {
		t.Errorf("BackupInfo = %+v, want exists with size and time", info)
	}
}

func TestExportZipCarriesAssetsAndImportRestoresThem(t *testing.T) {
	source, ctx, _ := newMediaService(t)

	// 给一个链接配上真实图标文件（走内容寻址存储）
	icon := testPNG(t, 32, 32)
	rel, err := source.Icons.Save(icon, "image/png")
	if err != nil {
		t.Fatalf("save icon: %v", err)
	}
	if _, err := source.PutBoard(ctx, testPage, &BoardPayload{
		Revision: 0,
		NewLinks: []LinkInput{{ID: "l1", URL: "https://github.com", Title: "GitHub"}},
		Items:    []ItemDTO{linkItem("i1", "l1", 0, 0)},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// PutBoard 之后手工把图标挂上（模拟 M5 抓取完成的状态）
	attachIcon(t, source, ctx, "l1", rel)

	zipData, err := source.ExportZip(ctx)
	if err != nil {
		t.Fatalf("ExportZip: %v", err)
	}
	if len(zipData) < 100 {
		t.Fatalf("zip 太小，可能没打包内容：%d 字节", len(zipData))
	}

	doc, assets, err := ParseImport("backup.zip", zipData)
	if err != nil {
		t.Fatalf("ParseImport: %v", err)
	}
	if len(assets) == 0 {
		t.Fatal("zip 里没有资源文件")
	}

	// 导入到一个**全新**的数据目录：图标必须靠 zip 里的资源还原
	target := newServiceInDir(t)
	if err := target.Import(ctx, doc, assets); err != nil {
		t.Fatalf("import into fresh service: %v", err)
	}

	exported, err := target.Export(ctx)
	if err != nil {
		t.Fatalf("Export target: %v", err)
	}
	if len(exported.Links) != 1 {
		t.Fatalf("links = %d, want 1", len(exported.Links))
	}
	link := exported.Links[0]
	if link.IconStatus != "ok" {
		t.Errorf("导入后图标状态 = %s, want ok（资源已随 zip 还原）", link.IconStatus)
	}
	if link.IconPath == nil {
		t.Fatal("icon_path 丢了")
	}
	full, err := target.Icons.Resolve(*link.IconPath)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := os.Stat(full); err != nil {
		t.Errorf("图标文件未还原：%v", err)
	}
}

func TestImportMarksMissingIconsAsMiss(t *testing.T) {
	svc, ctx, _ := newMediaService(t)
	seedRichState(t, svc, ctx)

	doc, err := svc.Export(ctx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	// 手工塞一个"指向不存在文件"的图标路径
	missing := "ff/ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff.png"
	doc.Links[0].IconPath = &missing
	doc.Links[0].IconStatus = "ok"

	// 导入到全新目录（没有那个文件）
	target := newServiceInDir(t)
	if err := target.Import(ctx, doc, nil); err != nil {
		t.Fatalf("Import: %v", err)
	}
	exported, err := target.Export(ctx)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	for _, l := range exported.Links {
		if l.IconPath != nil && *l.IconPath == missing && l.IconStatus != "miss" {
			t.Errorf("磁盘上不存在的图标应降级为 miss，得到 %s", l.IconStatus)
		}
	}
}

// newServiceInDir 建一个空库的 Service（独立的临时目录），用于验证"导入到新环境"。
func newServiceInDir(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	handle, err := db.Open(filepath.Join(dir, "nav.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	ctx := context.Background()
	if _, err := migrate.Run(ctx, handle, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// 同上：不注入 Fetcher，保持测试无网络依赖
	svc := New(handle, WithMedia(
		favicon.NewStore(filepath.Join(dir, "icons")),
		favicon.NewStore(filepath.Join(dir, "wallpapers", "orig")),
		favicon.NewStore(filepath.Join(dir, "wallpapers", "thumb")),
		nil,
		true,
	))
	svc.BackupDir = filepath.Join(dir, "backup")
	return svc
}

// attachIcon 把已落盘的图标挂到链接上（模拟抓取完成后的数据库状态）。
func attachIcon(t *testing.T, svc *Service, ctx context.Context, linkID, rel string) {
	t.Helper()
	link, err := svc.Q.GetLink(ctx, linkID)
	if err != nil {
		t.Fatalf("get link: %v", err)
	}
	mime := "image/png"
	link.IconPath = &rel
	link.IconMime = &mime
	link.IconStatus = "ok"
	if err := svc.Q.UpdateLink(ctx, dbgen.UpdateLinkParams{
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
		MonoText:      link.MonoText,
		MonoColor:     link.MonoColor,
		MonoFontSize:  link.MonoFontSize,
		ID:            linkID,
	}); err != nil {
		t.Fatalf("attach icon: %v", err)
	}
}
