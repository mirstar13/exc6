-- name: AddFriend :one
INSERT INTO friends (user_id, friend_id) 
VALUES ($1, $2)
RETURNING *;

-- name: AcceptFriend :one
UPDATE friends
SET accepted = true
WHERE user_id = $1 AND friend_id = $2
RETURNING *;

-- name: RemoveFreind :one
DELETE FROM friends 
WHERE user_id = $1 AND friend_id = $2
RETURNING *;

-- name: GetFriends :many
SELECT * FROM friends 
WHERE user_id = $1 AND accepted = true
OR friend_id = $1 AND accepted = true;

-- name: GetFriendRequests :many
SELECT * FROM friends 
WHERE friend_id = $1 AND accepted = false;