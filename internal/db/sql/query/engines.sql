-- engines.sql - search engines (keep this file pure ASCII, see 001_init.sql header)

-- name: ListEngines :many
SELECT * FROM engines ORDER BY sort_order, id;
