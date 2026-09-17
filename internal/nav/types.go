// Package nav 是 my_nav 的领域层：整板声明式布局、页面管理、不变量校验。
//
// 设计要点（对应 docs/spec.md §5.3 / §6.1）：
//   - 布局以"整板替换"提交，天然原子、幂等，前端可做乐观 UI。
//   - 校验是纯函数（ValidateBoard），不碰数据库，因此可以密集单测。
//   - 所有实体 id 由客户端生成（UUIDv7），服务端只做存在性与唯一性校验。
package nav

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	// GridCols 是逻辑网格列数（显示层按屏宽折行，见 spec §9）。
	GridCols = 12
	// MaxFolderItems 是单个文件夹的容量上限。
	MaxFolderItems = 9
	// MaxRows 是行数上限，防呆（避免脏数据把画布撑到天上去）。
	MaxRows = 200
)

// ---------- 读取模型（同时也是 JSON 契约，字段名需与 web/src/lib/types.ts 一致） ----------

type PageDTO struct {
	ID            string  `json:"id"`
	Slug          string  `json:"slug"`
	Name          string  `json:"name"`
	SortOrder     int64   `json:"sort_order"`
	Revision      int64   `json:"revision"`
	WallpaperMode string  `json:"wallpaper_mode"`
	WallpaperID   *string `json:"wallpaper_id"`
}

type LinkDTO struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	URL          string  `json:"url"`
	OpenNewTab   bool    `json:"open_new_tab"`
	IconSource   string  `json:"icon_source"`
	IconPath     *string `json:"icon_path"`
	IconStatus   string  `json:"icon_status"`
	MonoText     *string `json:"mono_text"`
	MonoColor    string  `json:"mono_color"`
	MonoFontSize int64   `json:"mono_font_size"`
}

type FolderDTO struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
	Size int64   `json:"size"`
}

type ChildDTO struct {
	ID        string `json:"id"`
	LinkID    string `json:"link_id"`
	SortOrder int64  `json:"sort_order"`
}

type ItemDTO struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"` // "link" | "folder"
	LinkID   string     `json:"link_id,omitempty"`
	FolderID string     `json:"folder_id,omitempty"`
	Size     int64      `json:"size,omitempty"`
	Col      int64      `json:"col"`
	Row      int64      `json:"row"`
	Children []ChildDTO `json:"children,omitempty"`
}

type Board struct {
	Page     PageDTO     `json:"page"`
	Revision int64       `json:"revision"`
	Items    []ItemDTO   `json:"items"`
	Links    []LinkDTO   `json:"links"`
	Folders  []FolderDTO `json:"folders"`
}

// ---------- 写入模型（PUT board 的请求体） ----------

type LinkInput struct {
	ID         string `json:"id"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	OpenNewTab *bool  `json:"open_new_tab,omitempty"`
}

type FolderInput struct {
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
	Size int64   `json:"size,omitempty"`
}

type BoardPayload struct {
	Revision         int64         `json:"revision"`
	Items            []ItemDTO     `json:"items"`
	NewLinks         []LinkInput   `json:"new_links"`
	NewFolders       []FolderInput `json:"new_folders"`
	DeletedLinkIDs   []string      `json:"deleted_link_ids"`
	DeletedFolderIDs []string      `json:"deleted_folder_ids"`
}

// ---------- 校验 ----------

// Conflict 是网格冲突位置，回给前端让它把被挤占的项顺延到最近空位后重试。
type Conflict struct {
	Col int64 `json:"col"`
	Row int64 `json:"row"`
}

// ValidationError 对应 HTTP 422。
type ValidationError struct {
	Message   string
	Conflicts []Conflict
}

func (e *ValidationError) Error() string { return e.Message }

// ValidateBoard 校验整板负载。knownLinks / knownFolders 是该页已有的实体 id 集合。
//
// 不变量（spec §5.3）：
//  1. 引用必须存在；不得引用本次要删除的实体
//  2. 每个链接在整个页面中最多出现一次（页面级或夹内，二者合计）
//  3. 文件夹容量 ≤ MaxFolderItems，且不允许嵌套（children 只能是链接）
//  4. 网格不越界、不重叠（2×2 夹占 4 格）
func ValidateBoard(p *BoardPayload, knownLinks, knownFolders map[string]struct{}) error {
	deletedLinks := toSet(p.DeletedLinkIDs)
	deletedFolders := toSet(p.DeletedFolderIDs)

	newLinks := make(map[string]struct{}, len(p.NewLinks))
	for i, l := range p.NewLinks {
		if l.ID == "" {
			return &ValidationError{Message: fmt.Sprintf("new_links[%d].id is required", i)}
		}
		if _, dup := newLinks[l.ID]; dup {
			return &ValidationError{Message: "duplicate new link id: " + l.ID}
		}
		if _, exists := knownLinks[l.ID]; exists {
			return &ValidationError{Message: "link already exists on this page: " + l.ID}
		}
		if !validHTTPURL(l.URL) {
			return &ValidationError{Message: fmt.Sprintf("new_links[%d].url must be an absolute http(s) URL: %q", i, l.URL)}
		}
		newLinks[l.ID] = struct{}{}
	}

	newFolders := make(map[string]struct{}, len(p.NewFolders))
	for i, f := range p.NewFolders {
		if f.ID == "" {
			return &ValidationError{Message: fmt.Sprintf("new_folders[%d].id is required", i)}
		}
		if _, dup := newFolders[f.ID]; dup {
			return &ValidationError{Message: "duplicate new folder id: " + f.ID}
		}
		if f.Size != 0 && f.Size != 1 && f.Size != 2 {
			return &ValidationError{Message: fmt.Sprintf("new_folders[%d].size must be 1 or 2", i)}
		}
		newFolders[f.ID] = struct{}{}
	}

	hasLink := func(id string) bool {
		if _, ok := newLinks[id]; ok {
			return true
		}
		_, ok := knownLinks[id]
		return ok
	}
	hasFolder := func(id string) bool {
		if _, ok := newFolders[id]; ok {
			return true
		}
		_, ok := knownFolders[id]
		return ok
	}

	occupied := make(map[Conflict]string)
	itemIDs := make(map[string]struct{}, len(p.Items))
	childIDs := make(map[string]struct{})
	usedLinks := make(map[string]struct{})

	for i, it := range p.Items {
		if it.ID == "" {
			return &ValidationError{Message: fmt.Sprintf("items[%d].id is required", i)}
		}
		if _, dup := itemIDs[it.ID]; dup {
			return &ValidationError{Message: "duplicate item id: " + it.ID}
		}
		itemIDs[it.ID] = struct{}{}

		span := int64(1)
		switch it.Kind {
		case "link":
			if it.LinkID == "" {
				return &ValidationError{Message: fmt.Sprintf("items[%d].link_id is required for kind=link", i)}
			}
			if _, gone := deletedLinks[it.LinkID]; gone {
				return &ValidationError{Message: "item references a link queued for deletion: " + it.LinkID}
			}
			if !hasLink(it.LinkID) {
				return &ValidationError{Message: "item references unknown link: " + it.LinkID}
			}
			if _, dup := usedLinks[it.LinkID]; dup {
				return &ValidationError{Message: "link appears more than once on this page: " + it.LinkID}
			}
			usedLinks[it.LinkID] = struct{}{}

		case "folder":
			if it.FolderID == "" {
				return &ValidationError{Message: fmt.Sprintf("items[%d].folder_id is required for kind=folder", i)}
			}
			if _, gone := deletedFolders[it.FolderID]; gone {
				return &ValidationError{Message: "item references a folder queued for deletion: " + it.FolderID}
			}
			if !hasFolder(it.FolderID) {
				return &ValidationError{Message: "item references unknown folder: " + it.FolderID}
			}
			switch it.Size {
			case 0, 1:
				span = 1
			case 2:
				span = 2
			default:
				return &ValidationError{Message: fmt.Sprintf("items[%d].size must be 1 or 2", i)}
			}
			if len(it.Children) > MaxFolderItems {
				return &ValidationError{
					Message: fmt.Sprintf("folder %s holds %d items, the limit is %d", it.FolderID, len(it.Children), MaxFolderItems),
				}
			}
			for j, c := range it.Children {
				if c.ID == "" {
					return &ValidationError{Message: fmt.Sprintf("items[%d].children[%d].id is required", i, j)}
				}
				if _, dup := childIDs[c.ID]; dup {
					return &ValidationError{Message: "duplicate child placement id: " + c.ID}
				}
				childIDs[c.ID] = struct{}{}
				if c.LinkID == "" {
					return &ValidationError{Message: fmt.Sprintf("items[%d].children[%d].link_id is required", i, j)}
				}
				if _, gone := deletedLinks[c.LinkID]; gone {
					return &ValidationError{Message: "folder child references a link queued for deletion: " + c.LinkID}
				}
				if !hasLink(c.LinkID) {
					return &ValidationError{Message: "folder child references unknown link: " + c.LinkID}
				}
				if _, dup := usedLinks[c.LinkID]; dup {
					return &ValidationError{Message: "link appears more than once on this page: " + c.LinkID}
				}
				usedLinks[c.LinkID] = struct{}{}
			}

		default:
			return &ValidationError{Message: fmt.Sprintf("items[%d].kind must be \"link\" or \"folder\", got %q", i, it.Kind)}
		}

		if it.Col < 0 || it.Row < 0 {
			return &ValidationError{Message: fmt.Sprintf("items[%d] has negative coordinates", i)}
		}
		if it.Col+span > GridCols {
			return &ValidationError{
				Message: fmt.Sprintf("items[%d] (%d wide) at col %d exceeds the %d-column grid", i, span, it.Col, GridCols),
			}
		}
		if it.Row+span > MaxRows {
			return &ValidationError{Message: fmt.Sprintf("items[%d] exceeds the maximum of %d rows", i, MaxRows)}
		}

		conflicts := make([]Conflict, 0, span*span)
		for dc := int64(0); dc < span; dc++ {
			for dr := int64(0); dr < span; dr++ {
				cell := Conflict{Col: it.Col + dc, Row: it.Row + dr}
				if _, taken := occupied[cell]; taken {
					conflicts = append(conflicts, cell)
					continue
				}
				occupied[cell] = it.ID
			}
		}
		if len(conflicts) > 0 {
			return &ValidationError{
				Message:   fmt.Sprintf("item %s overlaps existing items", it.ID),
				Conflicts: conflicts,
			}
		}
	}

	return nil
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}

func toSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			set[id] = struct{}{}
		}
	}
	return set
}
