-- name: CreateImplementation :one
INSERT INTO implementations (id, product_id, team_id, parent_implementation_id, name, is_active, inserted_at, updated_at)
VALUES (?, ?, ?, ?, ?, 1, ?, ?)
RETURNING *;

-- name: ListImplementationsByTeam :many
SELECT i.id, i.product_id, i.team_id, i.parent_implementation_id,
       i.name, i.description, i.is_active, i.inserted_at, i.updated_at,
       p.name AS product_name
FROM implementations i
JOIN products p ON p.id = i.product_id
WHERE i.team_id = ?
  AND i.is_active = 1
ORDER BY p.name, i.name;

-- name: ListImplementationsByProduct :many
SELECT i.id, i.product_id, i.team_id, i.parent_implementation_id,
       i.name, i.description, i.is_active, i.inserted_at, i.updated_at,
       p.name AS product_name
FROM implementations i
JOIN products p ON p.id = i.product_id
WHERE i.team_id = ?
  AND i.product_id = ?
  AND i.is_active = 1
ORDER BY i.name;

-- name: ListImplementationsByBranch :many
SELECT DISTINCT i.id, i.product_id, i.team_id, i.parent_implementation_id,
                i.name, i.description, i.is_active, i.inserted_at, i.updated_at,
                p.name AS product_name
FROM implementations i
JOIN products p ON p.id = i.product_id
JOIN tracked_branches tb ON tb.implementation_id = i.id
JOIN branches b ON b.id = tb.branch_id
WHERE i.team_id = ?
  AND i.is_active = 1
  AND b.repo_uri = ?
  AND b.branch_name = ?
ORDER BY p.name, i.name;

-- name: ListImplementationsByProductAndBranch :many
SELECT DISTINCT i.id, i.product_id, i.team_id, i.parent_implementation_id,
                i.name, i.description, i.is_active, i.inserted_at, i.updated_at,
                p.name AS product_name
FROM implementations i
JOIN products p ON p.id = i.product_id
JOIN tracked_branches tb ON tb.implementation_id = i.id
JOIN branches b ON b.id = tb.branch_id
WHERE i.team_id = ?
  AND i.product_id = ?
  AND i.is_active = 1
  AND b.repo_uri = ?
  AND b.branch_name = ?
ORDER BY i.name;
