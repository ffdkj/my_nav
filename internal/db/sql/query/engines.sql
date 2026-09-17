-- engines.sql - search engines (keep this file pure ASCII, see 001_init.sql header)

-- name: ListEngines :many
SELECT * FROM engines ORDER BY sort_order, id;

-- name: GetEngine :one
SELECT * FROM engines WHERE id = ?;

-- name: MaxEngineSortOrder :one
SELECT COALESCE(MAX(sort_order), 0) AS max_sort_order FROM engines;

-- name: CreateEngine :exec
INSERT INTO engines (id, name, url_tpl, icon_text, icon_color, sort_order, is_builtin)
VALUES (?, ?, ?, ?, ?, ?, 0);

-- name: UpdateEngine :exec
UPDATE engines SET name = ?, url_tpl = ?, icon_text = ?, icon_color = ?, sort_order = ? WHERE id = ?;

-- name: DeleteEngine :exec
DELETE FROM engines WHERE id = ?;
