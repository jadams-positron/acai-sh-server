-- name: CreateUser :one
INSERT INTO users(id, email, hashed_password, confirmed_at, inserted_at, updated_at)
VALUES (?,?,?,?,?,?)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = ? COLLATE NOCASE LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = ? LIMIT 1;

-- name: UpdateUserConfirmedAt :exec
UPDATE users SET confirmed_at=?, updated_at=? WHERE id=?;

-- name: MarkUserConfirmed :exec
UPDATE users
SET confirmed_at = ?, updated_at = ?
WHERE id = ? AND confirmed_at IS NULL;
