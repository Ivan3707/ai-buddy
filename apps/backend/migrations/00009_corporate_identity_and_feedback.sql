-- +goose Up
ALTER TABLE users ADD COLUMN corporate_id TEXT;
CREATE UNIQUE INDEX uq_users_corporate_id ON users(corporate_id) WHERE corporate_id IS NOT NULL;

CREATE TABLE checklist_feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    checklist_id UUID NOT NULL REFERENCES checklists(id),
    rating INTEGER NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment TEXT NOT NULL DEFAULT '',
    delivery_status TEXT NOT NULL DEFAULT 'pending' CHECK (delivery_status IN ('pending','sent','failed')),
    delivery_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at TIMESTAMPTZ,
    UNIQUE(user_id, checklist_id)
);
CREATE INDEX ix_checklist_feedback_delivery ON checklist_feedback(delivery_status, created_at) WHERE delivery_status <> 'sent';

CREATE TABLE mock_analytics_feedback (
    feedback_id UUID PRIMARY KEY,
    corporate_id TEXT NOT NULL,
    checklist_id UUID NOT NULL,
    checklist_type TEXT NOT NULL,
    rating INTEGER NOT NULL,
    comment TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE mock_analytics_feedback;
DROP TABLE checklist_feedback;
DROP INDEX uq_users_corporate_id;
ALTER TABLE users DROP COLUMN corporate_id;
