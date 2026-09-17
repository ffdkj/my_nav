package nav

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ffdkj/my_nav/internal/db/dbgen"
)

// ---------- 搜索引擎 ----------

// EngineDTO 是对外的引擎形状。
// 不直接回 dbgen.Engine：它的 is_builtin 是 int64（SQLite 没有布尔），
// 直接序列化会得到 1/0，前端就得用数字比较 —— 契约不干净。
type EngineDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URLTpl    string `json:"url_tpl"`
	IconText  string `json:"icon_text"`
	IconColor string `json:"icon_color"`
	SortOrder int64  `json:"sort_order"`
	IsBuiltin bool   `json:"is_builtin"`
}

func toEngineDTO(e dbgen.Engine) EngineDTO {
	return EngineDTO{
		ID:        e.ID,
		Name:      e.Name,
		URLTpl:    e.UrlTpl,
		IconText:  e.IconText,
		IconColor: e.IconColor,
		SortOrder: e.SortOrder,
		IsBuiltin: e.IsBuiltin != 0,
	}
}

func (s *Service) ListEngines(ctx context.Context) ([]EngineDTO, error) {
	rows, err := s.Q.ListEngines(ctx)
	if err != nil {
		return nil, fmt.Errorf("list engines: %w", err)
	}
	out := make([]EngineDTO, 0, len(rows))
	for _, e := range rows {
		out = append(out, toEngineDTO(e))
	}
	return out, nil
}

var allowedEngineColors = func() map[string]struct{} {
	m := map[string]struct{}{}
	for _, c := range []string{
		"#EF4444", "#F97316", "#EAB308", "#22C55E", "#14B8A6",
		"#06B6D4", "#3B82F6", "#6366F1", "#8B5CF6", "#EC4899",
	} {
		m[c] = struct{}{}
	}
	return m
}()

type EngineInput struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URLTpl    string `json:"url_tpl"`
	IconText  string `json:"icon_text"`
	IconColor string `json:"icon_color"`
}

// validateEngine 统一校验：模板必须含 {query}，否则用户搜不出东西。
func validateEngine(in EngineInput) *Error {
	if strings.TrimSpace(in.Name) == "" {
		return BadRequest("engine name is required")
	}
	if !strings.Contains(in.URLTpl, "{query}") {
		return BadRequest("url_tpl must contain the {query} placeholder")
	}
	if !strings.HasPrefix(in.URLTpl, "http://") && !strings.HasPrefix(in.URLTpl, "https://") {
		return BadRequest("url_tpl must be an absolute http(s) URL")
	}
	if len([]rune(in.IconText)) == 0 || len([]rune(in.IconText)) > 2 {
		return BadRequest("icon_text must be 1-2 characters")
	}
	if !validHexColor(in.IconColor) {
		return BadRequest("icon_color must be a #rrggbb value")
	}
	return nil
}

func (s *Service) CreateEngine(ctx context.Context, in EngineInput) (*EngineDTO, error) {
	if in.ID == "" {
		return nil, BadRequest("id is required")
	}
	if err := validateEngine(in); err != nil {
		return nil, err
	}
	max, err := s.Q.MaxEngineSortOrder(ctx)
	if err != nil {
		return nil, fmt.Errorf("max engine order: %w", err)
	}
	if err := s.Q.CreateEngine(ctx, dbgen.CreateEngineParams{
		ID:        in.ID,
		Name:      strings.TrimSpace(in.Name),
		UrlTpl:    in.URLTpl,
		IconText:  in.IconText,
		IconColor: in.IconColor,
		SortOrder: asInt64(max) + 10,
	}); err != nil {
		return nil, fmt.Errorf("create engine: %w", err)
	}
	engine, err := s.Q.GetEngine(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("reload engine: %w", err)
	}
	dto := toEngineDTO(engine)
	return &dto, nil
}

func (s *Service) UpdateEngine(ctx context.Context, id string, in EngineInput) (*EngineDTO, error) {
	current, err := s.Q.GetEngine(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("engine")
		}
		return nil, fmt.Errorf("get engine: %w", err)
	}
	// 内置引擎允许改名/改图标/改模板，但不许删（前端也据此禁用删除按钮）
	merged := EngineInput{
		ID:        id,
		Name:      firstNonEmpty(in.Name, current.Name),
		URLTpl:    firstNonEmpty(in.URLTpl, current.UrlTpl),
		IconText:  firstNonEmpty(in.IconText, current.IconText),
		IconColor: firstNonEmpty(in.IconColor, current.IconColor),
	}
	if err := validateEngine(merged); err != nil {
		return nil, err
	}
	if err := s.Q.UpdateEngine(ctx, dbgen.UpdateEngineParams{
		Name:      merged.Name,
		UrlTpl:    merged.URLTpl,
		IconText:  merged.IconText,
		IconColor: merged.IconColor,
		SortOrder: current.SortOrder,
		ID:        id,
	}); err != nil {
		return nil, fmt.Errorf("update engine: %w", err)
	}
	updated, err := s.Q.GetEngine(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reload engine: %w", err)
	}
	dto := toEngineDTO(updated)
	return &dto, nil
}

func (s *Service) DeleteEngine(ctx context.Context, id string) error {
	engine, err := s.Q.GetEngine(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return NotFound("engine")
		}
		return fmt.Errorf("get engine: %w", err)
	}
	if engine.IsBuiltin != 0 {
		return &Error{Status: 422, Code: "builtin_engine", Message: "built-in engines cannot be deleted"}
	}
	// 删掉的是当前默认引擎时，把默认值挪回首个内置引擎，避免搜索框失去默认
	settings, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	if settings["default_engine_id"] == id {
		if err := s.Q.UpsertSetting(ctx, dbgen.UpsertSettingParams{K: "default_engine_id", V: "eng-google"}); err != nil {
			return fmt.Errorf("reset default engine: %w", err)
		}
	}
	if err := s.Q.DeleteEngine(ctx, id); err != nil {
		return fmt.Errorf("delete engine: %w", err)
	}
	return nil
}

// ReorderEngines 按给定顺序重排（只动 sort_order，不删不建）。
func (s *Service) ReorderEngines(ctx context.Context, orderedIDs []string) error {
	if len(orderedIDs) == 0 {
		return BadRequest("ids are required")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := dbgen.New(tx)
	for i, id := range orderedIDs {
		engine, err := q.GetEngine(ctx, id)
		if err != nil {
			return NotFound("engine " + id)
		}
		if err := q.UpdateEngine(ctx, dbgen.UpdateEngineParams{
			Name:      engine.Name,
			UrlTpl:    engine.UrlTpl,
			IconText:  engine.IconText,
			IconColor: engine.IconColor,
			SortOrder: int64((i + 1) * 10),
			ID:        id,
		}); err != nil {
			return fmt.Errorf("reorder engine %s: %w", id, err)
		}
	}
	return tx.Commit()
}

// asInt64 处理 sqlc 对 COALESCE(MAX(...)) 推断出的 interface{} 返回类型。
func asInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
