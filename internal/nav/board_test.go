package nav

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ffdkj/my_nav/internal/db"
	"github.com/ffdkj/my_nav/internal/migrate"
)

const testPage = "page-home" // 迁移里种下的初始页

func newTestService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	handle, err := db.Open(filepath.Join(t.TempDir(), "nav.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	ctx := context.Background()
	if _, err := migrate.Run(ctx, handle, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(handle), ctx
}

func codeOf(t *testing.T, err error) *Error {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	apiErr := AsError(err)
	if apiErr == nil {
		t.Fatalf("expected *Error, got %T", err)
	}
	return apiErr
}

func linkItem(id, linkID string, col, row int64) ItemDTO {
	return ItemDTO{ID: id, Kind: "link", LinkID: linkID, Col: col, Row: row}
}

func TestPutBoardCreatesAndReadsBack(t *testing.T) {
	svc, ctx := newTestService(t)

	board, err := svc.PutBoard(ctx, testPage, &BoardPayload{
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
	})
	if err != nil {
		t.Fatalf("PutBoard: %v", err)
	}

	if board.Revision != 1 {
		t.Errorf("revision = %d, want 1", board.Revision)
	}
	if len(board.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(board.Items))
	}
	if len(board.Links) != 3 {
		t.Errorf("links = %d, want 3", len(board.Links))
	}
	if len(board.Folders) != 1 || board.Folders[0].Size != 2 {
		t.Errorf("folders = %+v, want one size-2 folder", board.Folders)
	}

	var folderItem *ItemDTO
	for i := range board.Items {
		if board.Items[i].Kind == "folder" {
			folderItem = &board.Items[i]
		}
	}
	if folderItem == nil {
		t.Fatal("folder item missing from board")
	}
	if folderItem.Size != 2 {
		t.Errorf("folder item size = %d, want 2", folderItem.Size)
	}
	if len(folderItem.Children) != 2 {
		t.Errorf("folder children = %d, want 2", len(folderItem.Children))
	}

	// 重新读一次，确认真的落库（而不是只在返回值里对）
	reread, err := svc.GetBoard(ctx, testPage)
	if err != nil {
		t.Fatalf("GetBoard: %v", err)
	}
	if reread.Revision != 1 || len(reread.Items) != 2 || len(reread.Folders) != 1 {
		t.Errorf("reread mismatch: %+v", reread)
	}
}

func TestPutBoardRejectsOverlap(t *testing.T) {
	svc, ctx := newTestService(t)

	_, err := svc.PutBoard(ctx, testPage, &BoardPayload{
		NewLinks: []LinkInput{
			{ID: "l1", URL: "https://a.dev"},
			{ID: "l2", URL: "https://b.dev"},
		},
		Items: []ItemDTO{
			linkItem("i1", "l1", 3, 3),
			linkItem("i2", "l2", 3, 3), // 同一格
		},
	})
	apiErr := codeOf(t, err)
	if apiErr.Status != 422 || apiErr.Code != "grid_conflict" {
		t.Fatalf("got status=%d code=%s, want 422/grid_conflict", apiErr.Status, apiErr.Code)
	}
	if len(apiErr.Conflicts) != 1 || apiErr.Conflicts[0].Col != 3 || apiErr.Conflicts[0].Row != 3 {
		t.Errorf("conflicts = %+v, want one cell at (3,3)", apiErr.Conflicts)
	}
}

func TestPutBoardRejectsBigFolderOverlap(t *testing.T) {
	svc, ctx := newTestService(t)

	// 2x2 文件夹占据 (0,0)-(1,1)；另一个链接放在 (1,1) 必然冲突
	_, err := svc.PutBoard(ctx, testPage, &BoardPayload{
		NewLinks:   []LinkInput{{ID: "l1", URL: "https://a.dev"}},
		NewFolders: []FolderInput{{ID: "f1", Size: 2}},
		Items: []ItemDTO{
			{ID: "i1", Kind: "folder", FolderID: "f1", Size: 2, Col: 0, Row: 0},
			linkItem("i2", "l1", 1, 1),
		},
	})
	apiErr := codeOf(t, err)
	if apiErr.Code != "grid_conflict" {
		t.Fatalf("code = %s, want grid_conflict (message: %s)", apiErr.Code, apiErr.Message)
	}
}

func TestPutBoardRejectsFolderCapacity(t *testing.T) {
	svc, ctx := newTestService(t)

	// 上限 9，第 10 个必须被拒
	links := make([]LinkInput, 0, 10)
	children := make([]ChildDTO, 0, 10)
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("l%d", i)
		links = append(links, LinkInput{ID: id, URL: fmt.Sprintf("https://s%d.dev", i)})
		children = append(children, ChildDTO{ID: fmt.Sprintf("c%d", i), LinkID: id, SortOrder: int64(i)})
	}

	_, err := svc.PutBoard(ctx, testPage, &BoardPayload{
		NewLinks:   links,
		NewFolders: []FolderInput{{ID: "f1", Size: 1}},
		Items: []ItemDTO{
			{ID: "i1", Kind: "folder", FolderID: "f1", Size: 1, Col: 0, Row: 0, Children: children},
		},
	})
	apiErr := codeOf(t, err)
	if apiErr.Status != 422 {
		t.Fatalf("status = %d, want 422 (message: %s)", apiErr.Status, apiErr.Message)
	}
}

func TestPutBoardRejectsOutOfBounds(t *testing.T) {
	svc, ctx := newTestService(t)

	// 12 列制：2x2 夹放在 col 11 会越界
	_, err := svc.PutBoard(ctx, testPage, &BoardPayload{
		NewFolders: []FolderInput{{ID: "f1", Size: 2}},
		Items: []ItemDTO{
			{ID: "i1", Kind: "folder", FolderID: "f1", Size: 2, Col: GridCols - 1, Row: 0},
		},
	})
	apiErr := codeOf(t, err)
	if apiErr.Status != 422 {
		t.Fatalf("status = %d, want 422", apiErr.Status)
	}
}

func TestPutBoardRejectsDuplicatedLink(t *testing.T) {
	svc, ctx := newTestService(t)

	// 同一个链接既在页面上、又在文件夹里 —— 属于脏数据，必须拒绝
	_, err := svc.PutBoard(ctx, testPage, &BoardPayload{
		NewLinks:   []LinkInput{{ID: "l1", URL: "https://a.dev"}},
		NewFolders: []FolderInput{{ID: "f1", Size: 1}},
		Items: []ItemDTO{
			linkItem("i1", "l1", 0, 0),
			{ID: "i2", Kind: "folder", FolderID: "f1", Size: 1, Col: 1, Row: 0, Children: []ChildDTO{
				{ID: "c1", LinkID: "l1", SortOrder: 0},
			}},
		},
	})
	apiErr := codeOf(t, err)
	if apiErr.Status != 422 {
		t.Fatalf("status = %d, want 422 (message: %s)", apiErr.Status, apiErr.Message)
	}
}

func TestPutBoardRevisionMismatch(t *testing.T) {
	svc, ctx := newTestService(t)

	// 先产生一次写，使 revision 变成 1
	if _, err := svc.PutBoard(ctx, testPage, &BoardPayload{Revision: 0}); err != nil {
		t.Fatalf("first PutBoard: %v", err)
	}

	// 再用过期的 revision=0 提交 → 409
	_, err := svc.PutBoard(ctx, testPage, &BoardPayload{Revision: 0})
	apiErr := codeOf(t, err)
	if apiErr.Status != 409 || apiErr.Code != "revision_mismatch" {
		t.Fatalf("got status=%d code=%s, want 409/revision_mismatch", apiErr.Status, apiErr.Code)
	}
}

func TestEmptyFolderIsAutoDeleted(t *testing.T) {
	svc, ctx := newTestService(t)

	// 建一个没有任何子项的文件夹：不变量要求它被自动清理
	board, err := svc.PutBoard(ctx, testPage, &BoardPayload{
		Revision:   0,
		NewFolders: []FolderInput{{ID: "f1", Size: 1}},
		Items: []ItemDTO{
			{ID: "i1", Kind: "folder", FolderID: "f1", Size: 1, Col: 0, Row: 0},
		},
	})
	if err != nil {
		t.Fatalf("PutBoard: %v", err)
	}
	if len(board.Folders) != 0 {
		t.Errorf("folders = %+v, want none (empty folder must be removed)", board.Folders)
	}
	if len(board.Items) != 0 {
		t.Errorf("items = %+v, want none (its placement cascades away)", board.Items)
	}
}

func TestDeleteLastPageIsRejected(t *testing.T) {
	svc, ctx := newTestService(t)

	err := svc.DeletePage(ctx, testPage)
	apiErr := codeOf(t, err)
	if apiErr.Code != "last_page" {
		t.Fatalf("code = %s, want last_page", apiErr.Code)
	}
}

func TestDeletePageCascadesPlacements(t *testing.T) {
	svc, ctx := newTestService(t)

	if _, err := svc.PutBoard(ctx, testPage, &BoardPayload{
		Revision: 0,
		NewLinks: []LinkInput{{ID: "l1", URL: "https://a.dev"}},
		Items:    []ItemDTO{linkItem("i1", "l1", 0, 0)},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}

	// 建第二个页面后删除第一个
	if _, err := svc.CreatePage(ctx, CreatePageInput{ID: "page-work", Slug: "work", Name: "Work"}); err != nil {
		t.Fatalf("create page: %v", err)
	}
	if err := svc.DeletePage(ctx, testPage); err != nil {
		t.Fatalf("delete page: %v", err)
	}

	if _, err := svc.GetBoard(ctx, testPage); err == nil {
		t.Fatal("expected the deleted page's board to be gone")
	} else {
		apiErr := AsError(err)
		if apiErr.Status != 404 {
			t.Fatalf("status = %d, want 404", apiErr.Status)
		}
	}

	// 链接实体保留（只有 placement 级联），符合 spec §5.3 第 5 条
	var links int
	if err := svc.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM links`).Scan(&links); err != nil {
		t.Fatalf("count links: %v", err)
	}
	if links != 1 {
		t.Errorf("links = %d, want 1 (link rows survive page deletion)", links)
	}

	var placements int
	if err := svc.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM placements`).Scan(&placements); err != nil {
		t.Fatalf("count placements: %v", err)
	}
	if placements != 0 {
		t.Errorf("placements = %d, want 0", placements)
	}
}

// ValidateBoard 是纯函数，直接用表驱动把边界情况钉死（不需要数据库）。
func TestValidateBoardTable(t *testing.T) {
	knownLinks := map[string]struct{}{"existing": {}}
	knownFolders := map[string]struct{}{}

	cases := []struct {
		name    string
		payload *BoardPayload
		wantErr bool
	}{
		{
			name:    "bad kind",
			payload: &BoardPayload{Items: []ItemDTO{{ID: "i1", Kind: "widget", Col: 0, Row: 0}}},
			wantErr: true,
		},
		{
			name: "negative coordinates",
			payload: &BoardPayload{
				NewLinks: []LinkInput{{ID: "l1", URL: "https://a.dev"}},
				Items:    []ItemDTO{linkItem("i1", "l1", -1, 0)},
			},
			wantErr: true,
		},
		{
			name: "unknown link reference",
			payload: &BoardPayload{
				Items: []ItemDTO{linkItem("i1", "nope", 0, 0)},
			},
			wantErr: true,
		},
		{
			name: "existing link is addressable",
			payload: &BoardPayload{
				Items: []ItemDTO{linkItem("i1", "existing", 0, 0)},
			},
			wantErr: false,
		},
		{
			name: "non http url",
			payload: &BoardPayload{
				NewLinks: []LinkInput{{ID: "l1", URL: "ftp://files.dev"}},
				Items:    []ItemDTO{linkItem("i1", "l1", 0, 0)},
			},
			wantErr: true,
		},
		{
			name: "duplicate item id",
			payload: &BoardPayload{
				NewLinks: []LinkInput{{ID: "l1", URL: "https://a.dev"}, {ID: "l2", URL: "https://b.dev"}},
				Items: []ItemDTO{
					linkItem("dup", "l1", 0, 0),
					linkItem("dup", "l2", 1, 0),
				},
			},
			wantErr: true,
		},
		{
			name: "two 2x2 folders side by side fit exactly",
			payload: &BoardPayload{
				NewFolders: []FolderInput{{ID: "f1", Size: 2}, {ID: "f2", Size: 2}},
				Items: []ItemDTO{
					{ID: "a", Kind: "folder", FolderID: "f1", Size: 2, Col: 0, Row: 0},
					{ID: "b", Kind: "folder", FolderID: "f2", Size: 2, Col: 2, Row: 0},
				},
			},
			wantErr: false,
		},
		{
			name: "row beyond max",
			payload: &BoardPayload{
				NewLinks: []LinkInput{{ID: "l1", URL: "https://a.dev"}},
				Items:    []ItemDTO{linkItem("i1", "l1", 0, MaxRows)},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBoard(tc.payload, knownLinks, knownFolders)
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
			if tc.wantErr {
				var valErr *ValidationError
				if !errors.As(err, &valErr) {
					t.Fatalf("error type = %T, want *ValidationError", err)
				}
			}
		})
	}
}
