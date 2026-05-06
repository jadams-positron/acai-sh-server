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

-- name: GetBranchByTeamRepoAndName :one
SELECT *
FROM branches
WHERE team_id = ? AND repo_uri = ? AND branch_name = ?
LIMIT 1;

-- name: CreateBranch :one
INSERT INTO branches (id, team_id, repo_uri, branch_name, last_seen_commit, inserted_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateBranchLastSeenCommit :exec
UPDATE branches
SET last_seen_commit = ?, updated_at = ?
WHERE id = ?;

-- name: UpsertSpec :one
-- Returns the inserted/updated spec row. The `xmax` trick used in Postgres
-- doesn't work in SQLite; we use the changes() function indirectly by checking
-- whether the row existed before. Caller decides created-vs-updated.
INSERT INTO specs (
  id, product_id, branch_id, path, last_seen_commit, parsed_at,
  feature_name, feature_description, feature_version, raw_content, requirements,
  inserted_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(branch_id, feature_name) DO UPDATE
SET path = excluded.path,
    last_seen_commit = excluded.last_seen_commit,
    parsed_at = excluded.parsed_at,
    feature_description = excluded.feature_description,
    feature_version = excluded.feature_version,
    raw_content = excluded.raw_content,
    requirements = excluded.requirements,
    updated_at = excluded.updated_at
RETURNING *;

-- name: UpsertFeatureBranchRef :exec
INSERT INTO feature_branch_refs (
  id, branch_id, feature_name, refs, "commit", pushed_at, inserted_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(branch_id, feature_name) DO UPDATE
SET refs = excluded.refs,
    "commit" = excluded."commit",
    pushed_at = excluded.pushed_at,
    updated_at = excluded.updated_at;

-- name: UpsertTrackedBranch :exec
INSERT INTO tracked_branches (implementation_id, branch_id, repo_uri, inserted_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(implementation_id, branch_id) DO NOTHING;
