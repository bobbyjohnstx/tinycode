-- name: GetProject :one
SELECT * FROM project WHERE id = ?;

-- name: GetProjectByWorktree :one
SELECT * FROM project WHERE worktree = ?;

-- name: CreateProject :exec
INSERT INTO project (id, worktree, name, sandboxes, time_created, time_updated)
VALUES (?, ?, ?, ?, ?, ?);

-- name: UpdateProject :exec
UPDATE project SET name = ?, time_updated = ? WHERE id = ?;

-- name: DeleteProject :exec
DELETE FROM project WHERE id = ?;
