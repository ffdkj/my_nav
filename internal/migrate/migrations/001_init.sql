-- 001_init.sql — my_nav 初始 schema（对应 docs/spec.md §5.2）
-- 约定：主键为客户端生成的 UUIDv7 文本；时间戳为 ISO8601 UTC。

CREATE TABLE pages (
  id             TEXT PRIMARY KEY,
  slug           TEXT NOT NULL UNIQUE,
  name           TEXT NOT NULL,
  sort_order     INTEGER NOT NULL,
  wallpaper_mode TEXT NOT NULL DEFAULT 'global' CHECK (wallpaper_mode IN ('global','custom')),
  wallpaper_id   TEXT,
  created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE links (
  id              TEXT PRIMARY KEY,
  title           TEXT NOT NULL,
  url             TEXT NOT NULL,
  open_new_tab    INTEGER NOT NULL DEFAULT 1,
  icon_source     TEXT NOT NULL DEFAULT 'auto' CHECK (icon_source IN ('auto','upload','monogram')),
  icon_path       TEXT,
  icon_mime       TEXT,
  icon_w          INTEGER,
  icon_h          INTEGER,
  icon_status     TEXT NOT NULL DEFAULT 'pending'
                    CHECK (icon_status IN ('pending','ok','miss','error')),
  icon_checked_at TEXT,
  mono_text       TEXT,
  mono_color      TEXT NOT NULL DEFAULT '#3B82F6',
  mono_font_size  INTEGER NOT NULL DEFAULT 30,
  created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE folders (
  id         TEXT PRIMARY KEY,
  name       TEXT,
  size       INTEGER NOT NULL DEFAULT 1 CHECK (size IN (1,2)),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- placements 是"位置"的唯一事实源：
--   in_folder IS NULL  -> 该 placement 位于页面上，位置由 (col,row) 决定
--   in_folder NOT NULL -> 该 placement 是某文件夹的子项，顺序由 sort_order 决定（col/row 无意义）
CREATE TABLE placements (
  id         TEXT PRIMARY KEY,
  page_id    TEXT NOT NULL REFERENCES pages(id)   ON DELETE CASCADE,
  folder_id  TEXT          REFERENCES folders(id) ON DELETE CASCADE,
  link_id    TEXT          REFERENCES links(id)   ON DELETE CASCADE,
  in_folder  TEXT          REFERENCES folders(id) ON DELETE CASCADE,
  col        INTEGER NOT NULL DEFAULT 0,
  row        INTEGER NOT NULL DEFAULT 0,
  sort_order INTEGER NOT NULL DEFAULT 0,
  CHECK ((link_id IS NULL) <> (folder_id IS NULL)),
  CHECK (folder_id IS NULL OR in_folder IS NULL)  -- 禁止文件夹嵌套
);

CREATE INDEX idx_place_page   ON placements(page_id, in_folder);
CREATE INDEX idx_place_folder ON placements(in_folder, sort_order);

CREATE TABLE engines (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  url_tpl    TEXT NOT NULL,
  icon_text  TEXT NOT NULL,
  icon_color TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  is_builtin INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE wallpapers (
  id         TEXT PRIMARY KEY,
  kind       TEXT NOT NULL CHECK (kind IN ('upload','url')),
  remote_url TEXT,
  file       TEXT,
  thumb_file TEXT,
  w          INTEGER,
  h          INTEGER,
  bytes      INTEGER,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE settings (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL
);

-- 内置搜索引擎（spec §1 决策 16；id 固定，便于导入导出对齐）
INSERT INTO engines (id, name, url_tpl, icon_text, icon_color, sort_order, is_builtin) VALUES
  ('eng-google', '谷歌',   'https://www.google.com/search?q={query}', 'G',  '#4285F4', 10, 1),
  ('eng-bing',   '必应',   'https://www.bing.com/search?q={query}',   'b',  '#0F7B6C', 20, 1),
  ('eng-baidu',  '百度',   'https://www.baidu.com/s?wd={query}',      '百', '#2932E1', 30, 1),
  ('eng-ddgo',   'DuckDuckGo', 'https://duckduckgo.com/?q={query}',   'D',  '#DE5833', 40, 1),
  ('eng-sogou',  '搜狗',   'https://www.sogou.com/web?query={query}', 'S',  '#FD6C1C', 50, 1),
  ('eng-youdao', '有道',   'https://www.youdao.com/result?word={query}', '有', '#D93B3B', 60, 1);

-- 默认设置
INSERT INTO settings (k, v) VALUES
  ('default_engine_id',    'eng-google'),
  ('merge_dwell_ms',       '500'),
  ('page_flip_edge_ms',    '150'),
  ('wallpaper_mode',       'global'),
  ('wallpaper_id',         ''),
  ('wallpaper_rotation',   'off'),
  ('wallpaper_interval_min','30'),
  ('wallpaper_fallback',   '#0b1220'),
  ('search_open_new_tab',  '1'),
  ('theme',                'dark');

-- 一个初始页面（首次运行的落地页）
INSERT INTO pages (id, slug, name, sort_order, wallpaper_mode) VALUES
  ('page-home', 'home', '主页', 0, 'global');
