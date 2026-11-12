-- +goose Up
ALTER TABLE checklist_items ADD COLUMN assigned_user_id UUID REFERENCES users(id);
ALTER TABLE user_checklist_progress ADD COLUMN position INTEGER,
    ADD COLUMN activated_at TIMESTAMPTZ;
UPDATE user_checklist_progress p SET position = i.position,
    activated_at = CASE WHEN p.status = 'active' THEN p.created_at ELSE NULL END
    FROM checklist_items i WHERE i.id = p.checklist_item_id;
ALTER TABLE user_checklist_progress ALTER COLUMN position SET NOT NULL;
CREATE INDEX progress_user_checklist_position_idx
    ON user_checklist_progress(user_id, checklist_id, position);
CREATE TABLE task_hints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    progress_id UUID NOT NULL UNIQUE REFERENCES user_checklist_progress(id) ON DELETE CASCADE,
    text TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','ready')),
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX task_hints_due_idx ON task_hints(status, next_attempt_at);

-- +goose Down
DROP TABLE task_hints;
DROP INDEX progress_user_checklist_position_idx;
ALTER TABLE user_checklist_progress DROP COLUMN position, DROP COLUMN activated_at;
ALTER TABLE checklist_items DROP COLUMN assigned_user_id;
