-- settings.sql - key/value settings (keep this file pure ASCII, see 001_init.sql header)

-- name: ListSettings :many
SELECT k, v FROM settings;

-- name: UpsertSetting :exec
INSERT INTO settings (k, v) VALUES (?, ?)
ON CONFLICT(k) DO UPDATE SET v = excluded.v;
