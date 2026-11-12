-- +goose Up
ALTER TABLE users ADD COLUMN first_name TEXT, ADD COLUMN last_name TEXT,
    ADD COLUMN telegram_username TEXT, ADD COLUMN department TEXT, ADD COLUMN team TEXT;

CREATE TABLE auth_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX auth_sessions_user_id_idx ON auth_sessions(user_id);

-- +goose Down
DROP TABLE auth_sessions;
ALTER TABLE users DROP COLUMN first_name, DROP COLUMN last_name,
    DROP COLUMN telegram_username, DROP COLUMN department, DROP COLUMN team;
