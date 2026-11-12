-- +goose Up
ALTER TABLE users DROP CONSTRAINT ck_users_role;
ALTER TABLE users ADD CONSTRAINT ck_users_role CHECK (role IN ('intern','buddy','teamlead'));
ALTER TABLE users ADD COLUMN teamlead_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN birth_date DATE,
    ADD COLUMN employed_since DATE;
CREATE INDEX ix_users_teamlead_id ON users(teamlead_id);

CREATE TABLE corporate_access_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tool TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    ticket_number TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','approved','denied')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id,idempotency_key)
);
CREATE INDEX ix_corporate_access_requests_user ON corporate_access_requests(user_id,created_at);

-- +goose Down
DROP TABLE corporate_access_requests;
DROP INDEX ix_users_teamlead_id;
ALTER TABLE users DROP COLUMN teamlead_id, DROP COLUMN birth_date, DROP COLUMN employed_since;
ALTER TABLE users DROP CONSTRAINT ck_users_role;
ALTER TABLE users ADD CONSTRAINT ck_users_role CHECK (role IN ('intern','buddy'));
