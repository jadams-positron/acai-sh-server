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

-- name: GetTeamByName :one
SELECT *
FROM teams
WHERE name = ? COLLATE NOCASE
LIMIT 1;

-- name: ListMembersForTeam :many
SELECT u.id AS user_id, u.email AS user_email, utr.title AS role_title,
       utr.inserted_at AS joined_at
FROM user_team_roles utr
JOIN users u ON u.id = utr.user_id
WHERE utr.team_id = ?
ORDER BY utr.inserted_at;

-- name: GetUserTeamRole :one
SELECT *
FROM user_team_roles
WHERE team_id = ? AND user_id = ?
LIMIT 1;
