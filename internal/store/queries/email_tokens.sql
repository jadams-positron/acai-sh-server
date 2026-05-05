-- name: CreateEmailToken :one
INSERT INTO email_tokens (id, user_id, token_hash, context, sent_to, inserted_at)
VALUES (?,?,?,?,?,?)
RETURNING *;

-- name: GetEmailTokenByHashAndContext :one
SELECT * FROM email_tokens WHERE token_hash = ? AND context = ? LIMIT 1;

-- name: DeleteEmailToken :exec
DELETE FROM email_tokens WHERE id = ?;

-- name: DeleteEmailTokensForUser :exec
DELETE FROM email_tokens WHERE user_id = ? AND context = ?;
