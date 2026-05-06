-- name: GetTeamByID :one
SELECT *
FROM teams
WHERE id = ?
LIMIT 1;

-- name: CreateTeam :one
INSERT INTO teams (id, name, global_admin, inserted_at, updated_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: ListTeamsForUser :many
SELECT t.*
FROM teams t
JOIN user_team_roles utr ON utr.team_id = t.id
WHERE utr.user_id = ?
ORDER BY t.name;

-- name: CreateUserTeamRole :exec
INSERT INTO user_team_roles (team_id, user_id, title, inserted_at, updated_at)
VALUES (?, ?, ?, ?, ?);
