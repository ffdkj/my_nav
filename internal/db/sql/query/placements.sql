-- placements.sql - board placement CRUD (keep this file pure ASCII, see 001_init.sql header)

-- name: ListPlacementsForPage :many
SELECT * FROM placements WHERE page_id = ? ORDER BY in_folder IS NOT NULL, sort_order, row, col;

-- name: DeletePlacementsForPage :exec
DELETE FROM placements WHERE page_id = ?;

-- name: CreatePlacement :exec
INSERT INTO placements (id, page_id, folder_id, link_id, in_folder, col, row, sort_order)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: CountPlacementsInFolder :one
SELECT COUNT(*) FROM placements WHERE in_folder = ?;
