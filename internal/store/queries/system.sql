-- name: Ping :one
-- Ping returns 1 if the DB is reachable. Used by /_health.
SELECT 1 AS one;
