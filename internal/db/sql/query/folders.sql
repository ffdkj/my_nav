-- folders.sql - folder CRUD (keep this file pure ASCII, see 001_init.sql header)

-- name: CreateFolder :exec
INSERT INTO folders (id, name, size) VALUES (?, ?, ?);

-- name: UpdateFolder :exec
UPDATE folders SET name = ?, size = ? WHERE id = ?;

-- name: DeleteFolder :exec
DELETE FROM folders WHERE id = ?;

-- name: ListFoldersForPage :many
SELECT f.* FROM folders f
JOIN placements p ON p.folder_id = f.id
WHERE p.page_id = ?;

-- Folders that hold no items - used to enforce "empty folders are deleted".
-- name: ListEmptyFoldersForPage :many
SELECT f.id FROM folders f
JOIN placements p ON p.folder_id = f.id
WHERE p.page_id = ?
  AND NOT EXISTS (SELECT 1 FROM placements c WHERE c.in_folder = f.id);
