-- name: GetSpecByBranchAndFeature :one
SELECT *
FROM specs
WHERE branch_id = ?
  AND feature_name = ?
LIMIT 1;

-- name: GetFeatureBranchRef :one
SELECT *
FROM feature_branch_refs
WHERE branch_id = ?
  AND feature_name = ?
LIMIT 1;

-- name: GetFeatureImplState :one
SELECT *
FROM feature_impl_states
WHERE implementation_id = ?
  AND feature_name = ?
LIMIT 1;

-- name: GetImplementationByProductAndName :one
SELECT *
FROM implementations
WHERE product_id = ?
  AND name = ?
  AND is_active = 1
LIMIT 1;

-- name: ListBranchesForImplementation :many
SELECT b.*
FROM branches b
JOIN tracked_branches tb ON tb.branch_id = b.id
WHERE tb.implementation_id = ?
ORDER BY b.updated_at DESC;

-- name: PickRefsBranchForFeature :one
-- Of the impl's tracked branches, return the branch with the most-recent
-- feature_branch_refs.pushed_at for the given feature_name. Used as the
-- "refs_source" branch when we have multiple tracked branches.
SELECT b.*
FROM branches b
JOIN tracked_branches tb ON tb.branch_id = b.id
JOIN feature_branch_refs fbr ON fbr.branch_id = b.id AND fbr.feature_name = ?
WHERE tb.implementation_id = ?
ORDER BY fbr.pushed_at DESC
LIMIT 1;

-- name: ListSpecsForBranch :many
SELECT *
FROM specs
WHERE branch_id = ?
ORDER BY feature_name;

-- name: ListFeatureImplStatesForImpl :many
SELECT *
FROM feature_impl_states
WHERE implementation_id = ?
ORDER BY feature_name;

-- name: ListFeatureBranchRefsForBranch :many
SELECT *
FROM feature_branch_refs
WHERE branch_id = ?
ORDER BY feature_name;

-- name: UpsertFeatureImplState :exec
-- Upserts the row for (implementation_id, feature_name) with the given states JSON.
-- Insert if missing, update otherwise.
INSERT INTO feature_impl_states (id, implementation_id, feature_name, states, inserted_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(implementation_id, feature_name) DO UPDATE
SET states = excluded.states,
    updated_at = excluded.updated_at;
