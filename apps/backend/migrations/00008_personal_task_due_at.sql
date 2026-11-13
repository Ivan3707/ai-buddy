-- +goose Up
ALTER TABLE checklist_items ADD COLUMN due_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE checklist_items DROP COLUMN due_at;
