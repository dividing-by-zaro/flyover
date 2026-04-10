-- name: CreatePost :one
INSERT INTO posts (source_id, title, url, author, summary_short, summary_long, summary_status, tags, image_url, published_at, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetPost :one
SELECT p.*, s.name AS source_name, s.icon_url AS source_icon_url
FROM posts p
JOIN sources s ON p.source_id = s.id
WHERE p.id = $1;

-- name: PostExistsByURL :one
SELECT EXISTS(SELECT 1 FROM posts WHERE url = $1);

-- name: ListPosts :many
SELECT p.*, s.name AS source_name, s.icon_url AS source_icon_url
FROM posts p
JOIN sources s ON p.source_id = s.id
WHERE
    (sqlc.narg('source_id')::uuid IS NULL OR p.source_id = sqlc.narg('source_id')) AND
    (sqlc.narg('tag')::text IS NULL OR sqlc.narg('tag') = ANY(p.tags)) AND
    (sqlc.narg('before')::timestamptz IS NULL OR p.published_at < sqlc.narg('before')) AND
    (sqlc.narg('after')::timestamptz IS NULL OR p.published_at > sqlc.narg('after'))
ORDER BY p.published_at DESC
LIMIT $1 OFFSET $2;

-- name: SearchPosts :many
SELECT p.*, s.name AS source_name, s.icon_url AS source_icon_url,
    ts_rank(p.search_vector, websearch_to_tsquery('english', $1)) AS rank
FROM posts p
JOIN sources s ON p.source_id = s.id
WHERE p.search_vector @@ websearch_to_tsquery('english', $1)
    AND (sqlc.narg('source_id')::uuid IS NULL OR p.source_id = sqlc.narg('source_id'))
    AND (sqlc.narg('tag')::text IS NULL OR sqlc.narg('tag') = ANY(p.tags))
ORDER BY rank DESC
LIMIT $2 OFFSET $3;

-- name: UpdatePostSummary :exec
UPDATE posts
SET summary_short = $2,
    summary_long = $3,
    tags = $4,
    summary_status = 'ready',
    summarized_at = now()
WHERE id = $1;

-- name: UpdatePostSummaryFailed :exec
UPDATE posts
SET summary_status = CASE WHEN $4::text = 'permanent' THEN 'failed' ELSE 'pending' END,
    summary_attempts = summary_attempts + 1,
    summary_error = $2,
    summary_last_error_kind = $4,
    summary_next_attempt_at = $3
WHERE id = $1;

-- name: ListPendingSummaries :many
SELECT * FROM posts
WHERE summary_status = 'pending'
    AND (summary_next_attempt_at IS NULL OR summary_next_attempt_at <= now())
ORDER BY created_at ASC;

-- name: CountPosts :one
SELECT count(*) FROM posts;
