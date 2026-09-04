-- name: GetMessage :one
SELECT * FROM message WHERE id = ?;

-- name: ListMessagesBySession :many
SELECT * FROM message
WHERE session_id = ?
ORDER BY time_created ASC, id ASC;

-- name: CreateMessage :exec
INSERT INTO message (id, session_id, time_created, time_updated, data)
VALUES (?, ?, ?, ?, ?);

-- name: UpdateMessage :exec
UPDATE message SET data = ?, time_updated = ? WHERE id = ?;

-- name: DeleteMessagesBySession :exec
DELETE FROM message WHERE session_id = ?;
