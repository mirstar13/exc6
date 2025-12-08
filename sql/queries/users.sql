-- name: CreateUser :one
INSERT INTO users (username, password_hash, icon, custom_icon)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1;

-- name: GetAllUsernames :many
SELECT username FROM users;

-- name: UpdateUser :one 
UPDATE users
SET username = $2, updated_at = NOW(), icon = $3, custom_icon = $4
WHERE id = $1
RETURNING *;

-- name: DeleteUser :one
DELETE FROM users WHERE id = $1
RETURNING *;


