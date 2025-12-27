-- +goose NO TRANSACTION
-- +goose Up
-- Improve friend lookups (finding who added me)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_friends_friend_id ON friends(friend_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_friends_user_id_friend_id ON friends(user_id, friend_id);

-- Improve pending request lookups
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_friends_pending ON friends(friend_id) WHERE accepted = false;

-- Improve group member lookups
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_group_members_composite ON group_members(group_id, user_id);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_friends_friend_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_friends_user_id_friend_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_friends_pending;
DROP INDEX CONCURRENTLY IF EXISTS idx_group_members_composite;