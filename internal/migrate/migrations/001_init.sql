-- 001_init.sql - initial schema for my_nav (see docs/spec.md 5.2)
--
-- CONVENTION: keep this file pure ASCII.
-- sqlc v1.31.1's SQLite parser silently truncates tokens when the .sql file
-- contains multi-byte UTF-8 characters (verified: 1072 non-ASCII bytes -> 8 bogus
-- syntax errors; 0 bytes -> clean generation). Put prose in Go comments instead.
--
-- Primary keys are client-generated UUIDv7 text; timestamps are ISO8601 UTC.

CREATE TABLE pages (
  id             TEXT PRIMARY KEY,
  slug           TEXT NOT NULL UNIQUE,
  name           TEXT NOT NULL,
  sort_order     INTEGER NOT NULL,
  -- optimistic concurrency for declarative whole-board PUT: client sends the
  -- revision it read; a mismatch is rejected with 409
  revision       INTEGER NOT NULL DEFAULT 0,
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

-- placements is the single source of truth for "where something sits":
--   in_folder IS NULL  -> the item sits on a page; position is (col,row)
--   in_folder NOT NULL -> the item lives inside that folder; order is sort_order
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
  CHECK (folder_id IS NULL OR in_folder IS NULL)  -- folders can never nest
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

-- Built-in search engines (ids are stable so export/import can reference them)
INSERT INTO engines (id, name, url_tpl, icon_text, icon_color, sort_order, is_builtin) VALUES
  ('eng-google', 'Google',     'https://www.google.com/search?q={query}', 'G', '#4285F4', 10, 1),
  ('eng-bing',   'Bing',       'https://www.bing.com/search?q={query}',   'b', '#0F7B6C', 20, 1),
  ('eng-baidu',  'Baidu',      'https://www.baidu.com/s?wd={query}',      'B', '#2932E1', 30, 1),
  ('eng-ddgo',   'DuckDuckGo', 'https://duckduckgo.com/?q={query}',       'D', '#DE5833', 40, 1),
  ('eng-sogou',  'Sogou',      'https://www.sogou.com/web?query={query}', 'S', '#FD6C1C', 50, 1),
  ('eng-youdao', 'Youdao',     'https://www.youdao.com/result?word={query}', 'Y', '#D93B3B', 60, 1);

-- Default settings
INSERT INTO settings (k, v) VALUES
  ('default_engine_id',     'eng-google'),
  ('merge_dwell_ms',        '500'),
  ('page_flip_edge_ms',     '150'),
  ('wallpaper_mode',        'global'),
  ('wallpaper_id',          ''),
  ('wallpaper_rotation',    'off'),
  ('wallpaper_interval_min','30'),
  ('wallpaper_fallback',    '#0b1220'),
  ('search_open_new_tab',   '1'),
  ('theme',                 'dark');

-- The landing page created on first run
INSERT INTO pages (id, slug, name, sort_order, wallpaper_mode) VALUES
  ('page-home', 'home', 'Home', 0, 'global');
