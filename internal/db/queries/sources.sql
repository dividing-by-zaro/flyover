-- name: ListSources :many
SELECT * FROM sources ORDER BY name;

-- name: GetSource :one
SELECT * FROM sources WHERE id = $1;

-- name: CreateSource :one
INSERT INTO sources (kind, name, site_url, feed_url, icon_url, metadata)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: DeleteSource :exec
DELETE FROM sources WHERE id = $1;

-- name: ListActiveRSSSources :many
SELECT * FROM sources WHERE kind = 'rss' AND is_active = true;

-- name: UpdateSourceLastPolled :exec
UPDATE sources SET last_polled = now() WHERE id = $1;

-- name: UpdateSourceFeedURL :exec
UPDATE sources SET feed_url = $1 WHERE id = $2;
