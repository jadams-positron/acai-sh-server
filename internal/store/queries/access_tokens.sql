-- name: GetAccessTokenByPrefix :one
SELECT *
FROM access_tokens
WHERE token_prefix = ?
LIMIT 1;

-- name: UpdateAccessTokenLastUsed :exec
UPDATE access_tokens
SET last_used_at = ?, updated_at = ?
WHERE id = ?;

-- name: RevokeAccessToken :exec
UPDATE access_tokens
SET revoked_at = ?, updated_at = ?
WHERE id = ?;

-- name: ListAccessTokensForTeam :many
SELECT *
FROM access_tokens
WHERE team_id = ?
ORDER BY inserted_at DESC;

-- name: CreateAccessToken :one
INSERT INTO access_tokens (
  id, user_id, team_id, name, token_hash, token_prefix,
  scopes, expires_at, revoked_at, last_used_at, inserted_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;
