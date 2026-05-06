-- name: GetTeamByID :one
SELECT *
FROM teams
WHERE id = ?
LIMIT 1;

-- name: CreateTeam :one
INSERT INTO teams (id, name, global_admin, inserted_at, updated_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;
