-- name: LockState :one
SELECT state FROM line_state WHERE id = 1 FOR UPDATE;

-- name: ReadState :one
SELECT state FROM line_state WHERE id = 1;

-- name: SaveState :exec
UPDATE line_state SET state = $1 WHERE id = 1;
