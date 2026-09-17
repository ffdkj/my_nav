-- wallpapers.sql - wallpapers (keep this file pure ASCII, see 001_init.sql header)

-- name: ListWallpapers :many
SELECT * FROM wallpapers ORDER BY sort_order, created_at;

-- name: GetWallpaper :one
SELECT * FROM wallpapers WHERE id = ?;

-- name: MaxWallpaperSortOrder :one
SELECT COALESCE(MAX(sort_order), -1) AS max_sort_order FROM wallpapers;

-- name: CreateWallpaper :exec
INSERT INTO wallpapers (id, kind, remote_url, file, thumb_file, w, h, bytes, sort_order)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateWallpaperMeta :exec
UPDATE wallpapers SET sort_order = ?, remote_url = ? WHERE id = ?;

-- name: DeleteWallpaper :exec
DELETE FROM wallpapers WHERE id = ?;

-- name: MaterializeWallpaper :exec
UPDATE wallpapers
SET kind = 'upload', file = ?, thumb_file = ?, w = ?, h = ?, bytes = ?
WHERE id = ?;

-- name: CountWallpapersByFile :one
SELECT COUNT(*) FROM wallpapers WHERE file = ?;

-- name: CountWallpapersByThumb :one
SELECT COUNT(*) FROM wallpapers WHERE thumb_file = ?;
