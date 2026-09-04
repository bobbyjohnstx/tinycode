-- name: GetPermission :one
SELECT * FROM permission WHERE project_id = ?;

-- name: UpsertPermission :exec
INSERT INTO permission (project_id, time_created, time_updated, data)
VALUES (?, ?, ?, ?)
ON CONFLICT (project_id) DO UPDATE SET
    data = excluded.data,
    time_updated = excluded.time_updated;

-- name: DeletePermission :exec
DELETE FROM permission WHERE project_id = ?;
