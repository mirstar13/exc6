-- name: CreateGroup :one
INSERT INTO groups (name, description, icon, custom_icon, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetGroupByID :one
SELECT * FROM groups WHERE id = $1;

-- name: UpdateGroup :one
UPDATE groups
SET name = $2, description = $3, icon = $4, custom_icon = $5, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteGroup :one
DELETE FROM groups WHERE id = $1
RETURNING *;

-- name: GetUserGroups :many
SELECT g.* FROM groups g
INNER JOIN group_members gm ON g.id = gm.group_id
WHERE gm.user_id = $1
ORDER BY g.updated_at DESC;

-- name: AddGroupMember :one
INSERT INTO group_members (group_id, user_id, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: RemoveGroupMember :one
DELETE FROM group_members
WHERE group_id = $1 AND user_id = $2
RETURNING *;

-- name: GetGroupMembers :many
SELECT u.id, u.username, u.icon, u.custom_icon, gm.role, gm.joined_at
FROM group_members gm
INNER JOIN users u ON gm.user_id = u.id
WHERE gm.group_id = $1
ORDER BY gm.joined_at;

-- name: GetGroupMember :one
SELECT * FROM group_members
WHERE group_id = $1 AND user_id = $2;

-- name: UpdateMemberRole :one
UPDATE group_members
SET role = $3
WHERE group_id = $1 AND user_id = $2
RETURNING *;

-- name: IsGroupMember :one
SELECT EXISTS(
    SELECT 1 FROM group_members
    WHERE group_id = $1 AND user_id = $2
) AS is_member;

-- name: IsGroupAdmin :one
SELECT EXISTS(
    SELECT 1 FROM group_members
    WHERE group_id = $1 AND user_id = $2 AND role = 'admin'
) AS is_admin;

-- name: GetGroupMemberCount :one
SELECT COUNT(*) FROM group_members WHERE group_id = $1;