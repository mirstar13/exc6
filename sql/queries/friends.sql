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

-- name: GetFriendsWithDetails :many
SELECT u.id, u.username, u.icon, u.custom_icon, f.accepted, f.created_at
FROM friends f
JOIN users u ON 
    CASE 
        WHEN f.user_id = $1 THEN f.friend_id = u.id
        ELSE f.user_id = u.id
    END
WHERE (f.user_id = $1 OR f.friend_id = $1) 
AND f.accepted = true;

-- name: GetFriendRequests :many
SELECT * FROM friends 
WHERE friend_id = $1 AND accepted = false;