-- name: InsertEvent :exec
INSERT INTO events (
  id, team_id, product_id, impl_id, feature_name, actor_user_id,
  kind, payload, inserted_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListEventsForTeam :many
SELECT e.id, e.team_id, e.product_id, e.impl_id, e.feature_name,
       e.actor_user_id, e.kind, e.payload, e.inserted_at,
       u.email AS actor_email
FROM events e
LEFT JOIN users u ON u.id = e.actor_user_id
WHERE e.team_id = ?
ORDER BY e.inserted_at DESC
LIMIT ?;

-- name: ListEventsForProduct :many
SELECT e.id, e.team_id, e.product_id, e.impl_id, e.feature_name,
       e.actor_user_id, e.kind, e.payload, e.inserted_at,
       u.email AS actor_email
FROM events e
LEFT JOIN users u ON u.id = e.actor_user_id
WHERE e.product_id = ?
ORDER BY e.inserted_at DESC
LIMIT ?;

-- name: ListEventsForImpl :many
SELECT e.id, e.team_id, e.product_id, e.impl_id, e.feature_name,
       e.actor_user_id, e.kind, e.payload, e.inserted_at,
       u.email AS actor_email
FROM events e
LEFT JOIN users u ON u.id = e.actor_user_id
WHERE e.impl_id = ?
ORDER BY e.inserted_at DESC
LIMIT ?;
