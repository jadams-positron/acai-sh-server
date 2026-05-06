-- name: GetProductByTeamAndName :one
SELECT id, team_id, name, description, is_active, inserted_at, updated_at
FROM products
WHERE team_id = ?
  AND name = ? COLLATE NOCASE
LIMIT 1;
