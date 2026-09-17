-- links.sql - link CRUD (keep this file pure ASCII, see 001_init.sql header)

-- name: ListLinks :many
SELECT * FROM links ORDER BY title COLLATE NOCASE, id;

-- name: GetLink :one
SELECT * FROM links WHERE id = ?;

-- name: CreateLink :exec
INSERT INTO links (
  id, title, url, open_new_tab, icon_source, icon_path, icon_mime, icon_w, icon_h,
  icon_status, icon_checked_at, mono_text, mono_color, mono_font_size
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateLink :exec
UPDATE links
SET title = ?, url = ?, open_new_tab = ?, icon_source = ?, icon_path = ?, icon_mime = ?,
    icon_w = ?, icon_h = ?, icon_status = ?, icon_checked_at = ?, mono_text = ?,
    mono_color = ?, mono_font_size = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id = ?;

-- name: DeleteLink :exec
DELETE FROM links WHERE id = ?;

-- name: ListLinksForPage :many
SELECT l.* FROM links l
JOIN placements p ON p.link_id = l.id
WHERE p.page_id = ?;

-- name: ListAllLinksWithPage :many
SELECT l.*, p.id AS page_id, p.slug AS page_slug, p.name AS page_name
FROM links l
JOIN placements pl ON pl.link_id = l.id
JOIN pages p ON p.id = pl.page_id
ORDER BY l.title COLLATE NOCASE;

-- name: ClearLinkIcon :exec
UPDATE links
SET icon_path = NULL, icon_mime = NULL, icon_w = NULL, icon_h = NULL,
    icon_status = 'pending', icon_source = 'auto',
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id = ?;
