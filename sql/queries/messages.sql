-- name: CreateMessage :one
INSERT INTO messages (
    message_id,
    from_user_id,
    to_user_id,
    content,
    is_group,
    group_id
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetMessagesBetweenUsers :many
SELECT
    m.message_id,
    m.content,
    m.created_at,
    u_from.username as from_username,
    u_to.username as to_username
FROM messages m
JOIN users u_from ON m.from_user_id = u_from.id
JOIN users u_to ON m.to_user_id = u_to.id
WHERE
    (u_from.username = $1 AND u_to.username = $2) OR
    (u_from.username = $2 AND u_to.username = $1)
ORDER BY m.created_at DESC
LIMIT $3 OFFSET $4;
