-- +goose Up
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    username TEXT NOT NULL UNIQUE,
    role TEXT NOT NULL DEFAULT 'member',
    password_hash TEXT NOT NULL,
    icon TEXT,
    custom_icon TEXT
);

-- +goose Down
DROP TABLE users;