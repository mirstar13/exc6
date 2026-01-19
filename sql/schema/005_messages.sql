CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id VARCHAR(255) NOT NULL UNIQUE,
    from_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    to_user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    group_id UUID REFERENCES groups(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    is_group BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (
        (is_group = FALSE AND to_user_id IS NOT NULL) OR
        (is_group = TRUE AND group_id IS NOT NULL)
    )
);

CREATE INDEX idx_messages_users ON messages(from_user_id, to_user_id);
CREATE INDEX idx_messages_group ON messages(group_id);
CREATE INDEX idx_messages_created_at ON messages(created_at);
