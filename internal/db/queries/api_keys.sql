-- name: CreateAPIKey :one
INSERT INTO api_keys (key_hash, name)
VALUES ($1, $2)
RETURNING *;

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys WHERE key_hash = $1;

-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = now() WHERE id = $1;

-- name: CountAPIKeys :one
SELECT count(*) FROM api_keys;
