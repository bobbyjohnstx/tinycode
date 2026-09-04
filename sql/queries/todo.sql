-- name: ListTodosBySession :many
SELECT * FROM todo
WHERE session_id = ?
ORDER BY position ASC;

-- name: UpsertTodo :exec
INSERT INTO todo (session_id, content, status, priority, position, time_created, time_updated)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (session_id, position) DO UPDATE SET
    content = excluded.content,
    status = excluded.status,
    priority = excluded.priority,
    time_updated = excluded.time_updated;

-- name: DeleteTodosBySession :exec
DELETE FROM todo WHERE session_id = ?;
