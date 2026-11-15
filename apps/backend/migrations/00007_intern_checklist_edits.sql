-- +goose Up
-- Per-intern edits of a checklist made by the intern's teamlead (docs/intern-checklist-edits.md).
-- The catalog (checklist_items with assigned_user_id IS NULL) is shared by all interns and stays untouched:
-- a teamlead's change is stored on the intern's own progress row.
--   hidden_at                 the task is removed from this intern's checklist (soft delete: submissions,
--                             comments and hints keep their foreign keys; NULL = visible);
--   *_override                the intern's own wording / estimate / deadline; NULL = the catalog value.
-- Additive only: existing rows get NULLs and behave exactly as before.
ALTER TABLE public.user_checklist_progress
    ADD COLUMN hidden_at timestamp with time zone,
    ADD COLUMN title_override character varying(255),
    ADD COLUMN description_override text,
    ADD COLUMN expected_result_override text,
    ADD COLUMN estimated_minutes_override integer,
    ADD COLUMN due_at_override timestamp with time zone,
    ADD CONSTRAINT chk_progress_title_override CHECK (title_override IS NULL OR btrim(title_override) <> ''),
    ADD CONSTRAINT chk_progress_estimated_minutes_override CHECK (estimated_minutes_override IS NULL OR estimated_minutes_override > 0);

-- +goose Down
ALTER TABLE public.user_checklist_progress
    DROP CONSTRAINT chk_progress_estimated_minutes_override,
    DROP CONSTRAINT chk_progress_title_override,
    DROP COLUMN due_at_override,
    DROP COLUMN estimated_minutes_override,
    DROP COLUMN expected_result_override,
    DROP COLUMN description_override,
    DROP COLUMN title_override,
    DROP COLUMN hidden_at;
