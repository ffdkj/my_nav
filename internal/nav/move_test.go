package nav

import (
	"context"
	"testing"
)

func seedTwoPages(t *testing.T) (*Service, context.Context, string) {
	t.Helper()
	svc, ctx := newTestService(t)

	// 初始页 page-home 上放两个链接
	if _, err := svc.PutBoard(ctx, testPage, &BoardPayload{
		Revision: 0,
		NewLinks: []LinkInput{
			{ID: "l1", URL: "https://a.dev", Title: "A"},
			{ID: "l2", URL: "https://b.dev", Title: "B"},
		},
		Items: []ItemDTO{linkItem("i1", "l1", 0, 0), linkItem("i2", "l2", 1, 0)},
	}); err != nil {
		t.Fatalf("seed board: %v", err)
	}
	if _, err := svc.CreatePage(ctx, CreatePageInput{ID: "page-work", Slug: "work", Name: "Work"}); err != nil {
		t.Fatalf("create page: %v", err)
	}
	return svc, ctx, "page-work"
}

func TestMoveItemAcrossPages(t *testing.T) {
	svc, ctx, workPage := seedTwoPages(t)

	board, err := svc.MoveItem(ctx, MoveItemInput{
		FromPageID: testPage,
		ToPageID:   workPage,
		ItemID:     "i1",
		Items:      []ItemDTO{linkItem("i1", "l1", 3, 0)},
	})
	if err != nil {
		t.Fatalf("MoveItem: %v", err)
	}
	if len(board.Items) != 1 || board.Items[0].ID != "i1" || board.Items[0].Col != 3 {
		t.Fatalf("target board = %+v, want i1 at col 3", board.Items)
	}
	if len(board.Links) != 1 {
		t.Errorf("target links = %d, want 1", len(board.Links))
	}

	source, err := svc.GetBoard(ctx, testPage)
	if err != nil {
		t.Fatalf("GetBoard source: %v", err)
	}
	if len(source.Items) != 1 || source.Items[0].ID != "i2" {
		t.Fatalf("source board = %+v, want only i2 left", source.Items)
	}
	// 两个页面的 revision 都要 +1，否则前端拿着旧 revision 提交会 409。
	// 源页：seed 时 0->1，搬走时 1->2；目标页：0->1。
	if source.Revision != 2 {
		t.Errorf("source revision = %d, want 2", source.Revision)
	}
	if board.Page.Revision != 1 {
		t.Errorf("target revision = %d, want 1", board.Page.Revision)
	}
}

func TestMoveItemRejectsOccupiedCell(t *testing.T) {
	svc, ctx, workPage := seedTwoPages(t)

	// 目标页先占住 (0,0)
	if _, err := svc.PutBoard(ctx, workPage, &BoardPayload{
		Revision: 0,
		NewLinks: []LinkInput{{ID: "l9", URL: "https://z.dev", Title: "Z"}},
		Items:    []ItemDTO{linkItem("i9", "l9", 0, 0)},
	}); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	// 客户端算错落点（与目标页已有的 i9 重叠）时必须被服务端拒绝
	_, err := svc.MoveItem(ctx, MoveItemInput{
		FromPageID: testPage,
		ToPageID:   workPage,
		ItemID:     "i1",
		Items:      []ItemDTO{linkItem("i1", "l1", 0, 0), linkItem("i9", "l9", 0, 0)},
	})
	apiErr := codeOf(t, err)
	if apiErr.Status != 422 || apiErr.Code != "grid_conflict" {
		t.Fatalf("got status=%d code=%s, want 422/grid_conflict", apiErr.Status, apiErr.Code)
	}
}

func TestMoveItemMovesFolderChildren(t *testing.T) {
	svc, ctx, workPage := seedTwoPages(t)

	// 把两个链接合并成一个夹
	board, err := svc.PutBoard(ctx, testPage, &BoardPayload{
		Revision:   1,
		NewFolders: []FolderInput{{ID: "f1", Size: 2}},
		Items: []ItemDTO{
			{
				ID: "if1", Kind: "folder", FolderID: "f1", Size: 2, Col: 0, Row: 0,
				Children: []ChildDTO{
					{ID: "c1", LinkID: "l1", SortOrder: 0},
					{ID: "c2", LinkID: "l2", SortOrder: 1},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("make folder: %v", err)
	}

	moved, err := svc.MoveItem(ctx, MoveItemInput{
		FromPageID: testPage,
		ToPageID:   workPage,
		ItemID:     "if1",
		Items: []ItemDTO{
			{
				ID: "if1", Kind: "folder", FolderID: "f1", Size: 2, Col: 0, Row: 0,
				Children: []ChildDTO{
					{ID: "c1", LinkID: "l1", SortOrder: 0},
					{ID: "c2", LinkID: "l2", SortOrder: 1},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("MoveItem folder: %v", err)
	}
	if len(moved.Items) != 1 || moved.Items[0].Kind != "folder" {
		t.Fatalf("target items = %+v, want the folder", moved.Items)
	}
	if len(moved.Items[0].Children) != 2 {
		t.Errorf("folder children on target page = %d, want 2 (children must follow)", len(moved.Items[0].Children))
	}

	source, err := svc.GetBoard(ctx, testPage)
	if err != nil {
		t.Fatalf("GetBoard source: %v", err)
	}
	if len(source.Items) != 0 {
		t.Errorf("source items = %d, want 0", len(source.Items))
	}
	_ = board
}

func TestMoveItemRejectsSamePageAndFolderChild(t *testing.T) {
	svc, ctx, workPage := seedTwoPages(t)

	if _, err := svc.MoveItem(ctx, MoveItemInput{FromPageID: testPage, ToPageID: testPage, ItemID: "i1"}); err == nil {
		t.Error("expected an error when moving to the same page")
	}
	if _, err := svc.MoveItem(ctx, MoveItemInput{FromPageID: testPage, ToPageID: workPage, ItemID: "nope"}); err == nil {
		t.Error("expected an error for an unknown item")
	}
	if _, err := svc.MoveItem(ctx, MoveItemInput{
		FromPageID: testPage, ToPageID: workPage, ItemID: "i1",
		Items: []ItemDTO{},
	}); err == nil {
		t.Error("expected an error when items omits the moved item")
	}
}
