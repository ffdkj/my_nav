-- pages.sql - page CRUD (keep this file pure ASCII, see 001_init.sql header)

-- name: ListPages :many
SELECT * FROM pages ORDER BY sort_order, id;

-- name: GetPage :one
SELECT * FROM pages WHERE id = ?;

-- name: GetPageBySlug :one
SELECT * FROM pages WHERE slug = ?;

-- name: CountPages :one
SELECT COUNT(*) FROM pages;

-- name: MaxPageSortOrder :one
SELECT COALESCE(MAX(sort_order), -1) AS max_sort_order FROM pages;

-- name: CreatePage :exec
INSERT INTO pages (id, slug, name, sort_order, wallpaper_mode, wallpaper_id)
VALUES (?, ?, ?, ?, ?, ?);

-- name: UpdatePage :exec
UPDATE pages
SET name = ?, slug = ?, wallpaper_mode = ?, wallpaper_id = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id = ?;

-- name: UpdatePageSortOrder :exec
UPDATE pages
SET sort_order = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id = ?;

-- name: BumpPageRevision :one
UPDATE pages
SET revision = revision + 1, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id = ?
RETURNING revision;

-- name: DeletePage :exec
DELETE FROM pages WHERE id = ?;
