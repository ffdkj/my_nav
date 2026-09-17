package nav

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ffdkj/my_nav/internal/db/dbgen"
)

// MoveItemInput 是跨页搬移的请求体。
// 客户端已经用打包器算好了目标页上的落点 (col,row)；服务端负责原子性与冲突校验。
type MoveItemInput struct {
	FromPageID string `json:"from_page_id"`
	ToPageID   string `json:"to_page_id"`
	ItemID     string `json:"item_id"`
	// Items 是**目标页搬完之后的完整布局**（由前端打包器算出）。
	// 只给一个落点是不够的：插入会把目标页原有图标挤开，
	// 服务端必须知道所有项的新坐标，才能在同一事务里既搬又重排。
	Items []ItemDTO `json:"items"`
}

// MoveItem 把一个页面级 placement（链接或文件夹）连同其夹内子项搬到另一个页面。
//
// 为什么要有这个专用端点：跨页拖拽若拆成"源页 PUT + 目标页 PUT"两次写，
// 中间失败会让图标凭空消失。这里用单事务保证要么完整搬过去、要么完全不动。
func (s *Service) MoveItem(ctx context.Context, in MoveItemInput) (*Board, error) {
	if in.FromPageID == "" || in.ToPageID == "" || in.ItemID == "" {
		return nil, BadRequest("from_page_id, to_page_id and item_id are required")
	}
	if in.FromPageID == in.ToPageID {
		return nil, BadRequest("source and target page are the same")
	}
	if _, err := s.Q.GetPage(ctx, in.FromPageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("source page")
		}
		return nil, fmt.Errorf("get source page: %w", err)
	}
	if _, err := s.Q.GetPage(ctx, in.ToPageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("target page")
		}
		return nil, fmt.Errorf("get target page: %w", err)
	}

	placement, err := s.Q.GetPlacement(ctx, in.ItemID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("item")
		}
		return nil, fmt.Errorf("get placement: %w", err)
	}
	if placement.PageID != in.FromPageID {
		return nil, BadRequest("item does not live on the source page")
	}
	if placement.InFolder != nil {
		return nil, BadRequest("only page-level items can be moved between pages")
	}

	// 找出被搬项在目标布局里的落点
	movedIndex := -1
	for i, it := range in.Items {
		if it.ID == in.ItemID {
			movedIndex = i
			break
		}
	}
	if movedIndex < 0 {
		return nil, BadRequest("items must contain the moved item")
	}
	movedItem := in.Items[movedIndex]

	// 校验目标布局：把被搬项携带的实体也算作"已知"，
	// 因为它们此刻还在源页上，但对目标页来说是合法的引用。
	targetLinks, err := s.Q.ListLinksForPage(ctx, in.ToPageID)
	if err != nil {
		return nil, fmt.Errorf("list target links: %w", err)
	}
	knownLinks := make(map[string]struct{}, len(targetLinks)+1)
	for _, l := range targetLinks {
		knownLinks[l.ID] = struct{}{}
	}
	targetFolders, err := s.Q.ListFoldersForPage(ctx, in.ToPageID)
	if err != nil {
		return nil, fmt.Errorf("list target folders: %w", err)
	}
	knownFolders := make(map[string]struct{}, len(targetFolders)+1)
	for _, f := range targetFolders {
		knownFolders[f.ID] = struct{}{}
	}
	switch {
	case movedItem.LinkID != "":
		knownLinks[movedItem.LinkID] = struct{}{}
	case movedItem.FolderID != "":
		knownFolders[movedItem.FolderID] = struct{}{}
		for _, c := range movedItem.Children {
			knownLinks[c.LinkID] = struct{}{}
		}
	}

	// 目标页现有 placement 的 id 集合：用于确认布局里没有凭空出现的项
	existing, err := s.Q.ListPlacementsForPage(ctx, in.ToPageID)
	if err != nil {
		return nil, fmt.Errorf("list target placements: %w", err)
	}
	pageLevel := make(map[string]struct{})
	childrenOf := make(map[string]struct{})
	for _, p := range existing {
		if p.InFolder == nil {
			pageLevel[p.ID] = struct{}{}
		} else {
			childrenOf[p.ID] = struct{}{}
		}
	}

	// 除被搬项外，布局里的每一项都必须已经是目标页上的页面级 placement
	for _, it := range in.Items {
		if it.ID == in.ItemID {
			continue
		}
		if _, ok := pageLevel[it.ID]; !ok {
			return nil, BadRequest("items contains an unknown placement: " + it.ID)
		}
	}

	if err := ValidateBoard(&BoardPayload{Items: in.Items}, knownLinks, knownFolders); err != nil {
		return nil, err
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := dbgen.New(tx)

	// 1) 把被搬项搬到目标页（文件夹还要带上夹内子项）
	if err := q.MovePlacement(ctx, dbgen.MovePlacementParams{
		PageID: in.ToPageID,
		Col:    movedItem.Col,
		Row:    movedItem.Row,
		ID:     in.ItemID,
	}); err != nil {
		return nil, fmt.Errorf("move placement: %w", err)
	}
	if movedItem.FolderID != "" {
		folderID := movedItem.FolderID
		if err := q.MoveFolderChildren(ctx, dbgen.MoveFolderChildrenParams{
			PageID:   in.ToPageID,
			InFolder: &folderID,
		}); err != nil {
			return nil, fmt.Errorf("move folder children: %w", err)
		}
	}

	// 2) 目标页其余项按客户端给的坐标重排（插入位置之后的项会整体后移）
	for _, it := range in.Items {
		if it.ID == in.ItemID {
			continue
		}
		if err := q.UpdatePlacementPos(ctx, dbgen.UpdatePlacementPosParams{
			Col: it.Col,
			Row: it.Row,
			ID:  it.ID,
		}); err != nil {
			return nil, fmt.Errorf("reposition %s: %w", it.ID, err)
		}
		if it.FolderID != "" && it.Size > 0 {
			if err := q.UpdateFolder(ctx, dbgen.UpdateFolderParams{
				Name: nil,
				Size: it.Size,
				ID:   it.FolderID,
			}); err != nil {
				return nil, fmt.Errorf("resize folder %s: %w", it.FolderID, err)
			}
		}
		for i, c := range it.Children {
			if err := q.SetPlacementSort(ctx, dbgen.SetPlacementSortParams{
				SortOrder: int64(i),
				ID:        c.ID,
			}); err != nil {
				return nil, fmt.Errorf("reorder child %s: %w", c.ID, err)
			}
		}
	}

	for _, pageID := range []string{in.FromPageID, in.ToPageID} {
		if _, err := q.BumpPageRevision(ctx, pageID); err != nil {
			return nil, fmt.Errorf("bump revision for %s: %w", pageID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit move: %w", err)
	}

	return s.GetBoard(ctx, in.ToPageID)
}

// occupancyConflicts 复用与整板校验相同的占位规则：
// 文件夹按其 size 占格（2x2 占 4 格），夹内子项不占页面格子。
func occupancyConflicts(placements []dbgen.Placement, folders map[string]int64, col, row, span int64) []Conflict {
	occupied := make(map[Conflict]struct{})
	for _, p := range placements {
		if p.InFolder != nil {
			continue
		}
		size := int64(1)
		if p.FolderID != nil {
			size = folders[*p.FolderID]
			if size == 0 {
				size = 1
			}
		}
		for dc := int64(0); dc < size; dc++ {
			for dr := int64(0); dr < size; dr++ {
				occupied[Conflict{Col: p.Col + dc, Row: p.Row + dr}] = struct{}{}
			}
		}
	}
	conflicts := make([]Conflict, 0)
	for dc := int64(0); dc < span; dc++ {
		for dr := int64(0); dr < span; dr++ {
			cell := Conflict{Col: col + dc, Row: row + dr}
			if _, taken := occupied[cell]; taken {
				conflicts = append(conflicts, cell)
			}
		}
	}
	return conflicts
}
