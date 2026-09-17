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

-- name: GetPlacement :one
SELECT * FROM placements WHERE id = ?;

-- name: MovePlacement :exec
UPDATE placements SET page_id = ?, col = ?, row = ? WHERE id = ?;

-- name: MoveFolderChildren :exec
UPDATE placements SET page_id = ? WHERE in_folder = ?;

-- name: UpdatePlacementPos :exec
UPDATE placements SET col = ?, row = ? WHERE id = ?;

-- name: SetPlacementSort :exec
UPDATE placements SET sort_order = ? WHERE id = ?;
