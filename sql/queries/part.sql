-- name: GetPart :one
SELECT * FROM part WHERE id = ?;

-- name: ListPartsByMessage :many
SELECT * FROM part
WHERE message_id = ?
ORDER BY id ASC;

-- name: ListPartsBySession :many
SELECT * FROM part
WHERE session_id = ?
ORDER BY time_created ASC, id ASC;

-- name: CreatePart :exec
INSERT INTO part (id, message_id, session_id, time_created, time_updated, data)
VALUES (?, ?, ?, ?, ?, ?);

-- name: UpdatePart :exec
UPDATE part SET data = ?, time_updated = ? WHERE id = ?;

-- name: DeletePartsByMessage :exec
DELETE FROM part WHERE message_id = ?;
