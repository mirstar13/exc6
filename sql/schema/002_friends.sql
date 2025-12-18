-- +goose Up
CREATE TABLE friends (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    friend_id UUID REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    accepted BOOL NOT NULL DEFAULT FALSE,
    UNIQUE (user_id, friend_id)
);

-- +goose Down
DROP TABLE friend;