-- 002_icon_pick.sql - hand-picked icon provenance + tile shape setting
--
-- Keep this file pure ASCII: sqlc 1.31.1's SQLite parser silently truncates
-- tokens on multi-byte UTF-8 in .sql files (same rule as 001_init.sql).
--
-- Two changes:
--   1) links.icon_picked_url - the remote URL of the candidate the user picked
--      by hand (NULL = never picked). The icon_source CHECK constraint stays
--      'auto'|'upload'|'monogram' (SQLite cannot alter a CHECK without
--      rebuilding the table), so provenance of a picked icon lives here and
--      icon_source remains 'auto'.
--   2) settings.tile_shape - global tile shape preset (rounded|circle|
--      squircle|square), validated in internal/nav/links.go.

ALTER TABLE links ADD COLUMN icon_picked_url TEXT;

INSERT OR IGNORE INTO settings (k, v) VALUES ('tile_shape', 'rounded');
