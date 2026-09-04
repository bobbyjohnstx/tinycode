-- name: GetSession :one
SELECT * FROM session WHERE id = ?;

-- name: ListSessionsByProject :many
SELECT * FROM session
WHERE project_id = ?
ORDER BY time_created DESC;

-- name: ListSessionsPaginated :many
SELECT * FROM session
WHERE project_id = ?
  AND (time_created < sqlc.arg(cursor_time) OR (time_created = sqlc.arg(cursor_time) AND id < sqlc.arg(cursor_id)))
ORDER BY time_created DESC, id DESC
LIMIT ?;

-- name: CreateSession :exec
INSERT INTO session (
    id, project_id, workspace_id, parent_id, slug, directory, path,
    title, version, agent, model, time_created, time_updated
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateSessionTitle :exec
UPDATE session SET title = ?, time_updated = ? WHERE id = ?;

-- name: UpdateSessionCost :exec
UPDATE session SET
    cost = cost + ?,
    tokens_input = tokens_input + ?,
    tokens_output = tokens_output + ?,
    tokens_reasoning = tokens_reasoning + ?,
    tokens_cache_read = tokens_cache_read + ?,
    tokens_cache_write = tokens_cache_write + ?,
    time_updated = ?
WHERE id = ?;

-- name: UpdateSessionSummary :exec
UPDATE session SET
    summary_additions = ?,
    summary_deletions = ?,
    summary_files = ?,
    summary_diffs = ?,
    time_updated = ?
WHERE id = ?;

-- name: ArchiveSession :exec
UPDATE session SET time_archived = ?, time_updated = ? WHERE id = ?;

-- name: DeleteSession :exec
DELETE FROM session WHERE id = ?;
