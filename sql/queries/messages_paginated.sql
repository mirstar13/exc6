-- name: GetMessagesBetweenUsersPaginated :many
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
    m.is_group = FALSE AND (
        (u_from.username = $1 AND u_to.username = $2) OR
        (u_from.username = $2 AND u_to.username = $1)
    ) AND m.created_at < $3
ORDER BY m.created_at DESC
LIMIT $4;

-- name: GetGroupMessagesPaginated :many
SELECT
    m.message_id,
    m.content,
    m.created_at,
    u_from.username as from_username,
    g.name as group_name
FROM messages m
JOIN users u_from ON m.from_user_id = u_from.id
JOIN groups g ON m.group_id = g.id
WHERE
    m.is_group = TRUE AND m.group_id = $1 AND m.created_at < $2
ORDER BY m.created_at DESC
LIMIT $3;
