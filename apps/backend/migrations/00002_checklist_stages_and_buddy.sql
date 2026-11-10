
-- +goose Up

-- Этапы внутри чек-листов.
CREATE TABLE checklist_stages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    checklist_id UUID NOT NULL REFERENCES checklists(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    week_start INTEGER NOT NULL CHECK (week_start >= 1),
    week_end INTEGER NOT NULL CHECK (week_end >= week_start),
    goal TEXT NOT NULL DEFAULT '',
    expected_result TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL CHECK (position >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_checklist_stages_position UNIQUE (checklist_id, position),
    CONSTRAINT uq_checklist_stages_id_checklist UNIQUE (id, checklist_id)
);

-- Сначала добавляем nullable stage_id, чтобы перенести существующие задания.
ALTER TABLE checklist_items
    ADD COLUMN stage_id UUID;

-- Для старых заданий создаём технический этап.
INSERT INTO checklist_stages (
    checklist_id,
    title,
    week_start,
    week_end,
    goal,
    expected_result,
    position
)
SELECT DISTINCT
    ci.checklist_id,
    'Основной этап',
    1,
    1,
    '',
    '',
    1
FROM checklist_items ci
WHERE NOT EXISTS (
    SELECT 1
    FROM checklist_stages cs
    WHERE cs.checklist_id = ci.checklist_id
);

-- Привязываем старые задания к созданному этапу.
UPDATE checklist_items ci
SET stage_id = cs.id
FROM checklist_stages cs
WHERE cs.checklist_id = ci.checklist_id
  AND cs.position = 1
  AND ci.stage_id IS NULL;

ALTER TABLE checklist_items
    ALTER COLUMN stage_id SET NOT NULL;

ALTER TABLE checklist_items
    ADD CONSTRAINT fk_checklist_items_stage
    FOREIGN KEY (stage_id, checklist_id)
    REFERENCES checklist_stages (id, checklist_id)
    ON DELETE CASCADE;

ALTER TABLE checklist_items
    ADD COLUMN expected_result TEXT,
    ADD COLUMN requires_review BOOLEAN NOT NULL DEFAULT FALSE;

-- Разрешаем новый статус задания: verified.
-- +goose StatementBegin
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'user_checklist_progress'::regclass
          AND contype = 'c'
          AND pg_get_constraintdef(oid) ILIKE '%status%'
    LOOP
        EXECUTE format(
            'ALTER TABLE user_checklist_progress DROP CONSTRAINT %I',
            constraint_name
        );
    END LOOP;
END
$$;
-- +goose StatementEnd

ALTER TABLE user_checklist_progress
    ADD CONSTRAINT ck_user_checklist_progress_status
    CHECK (status IN ('unavailable', 'active', 'completed', 'verified'));

-- Buddy заменяет teamlead. Сохраняем существующих пользователей.
-- +goose StatementBegin
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'users'::regclass
          AND contype = 'c'
          AND pg_get_constraintdef(oid) ILIKE '%teamlead%'
    LOOP
        EXECUTE format(
            'ALTER TABLE users DROP CONSTRAINT %I',
            constraint_name
        );
    END LOOP;
END
$$;
-- +goose StatementEnd

UPDATE users
SET role = 'buddy'
WHERE role = 'teamlead';

ALTER TABLE users
    ADD CONSTRAINT ck_users_role
    CHECK (role IN ('intern', 'buddy'));

-- Основной buddy стажёра.
ALTER TABLE users
    ADD COLUMN buddy_id UUID NULL REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX ix_users_buddy_id ON users(buddy_id);


-- +goose Down

DROP INDEX IF EXISTS ix_users_buddy_id;

ALTER TABLE users
    DROP COLUMN IF EXISTS buddy_id;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS ck_users_role;

UPDATE users
SET role = 'teamlead'
WHERE role = 'buddy';

ALTER TABLE users
    ADD CONSTRAINT ck_users_role
    CHECK (role IN ('intern', 'teamlead'));

ALTER TABLE user_checklist_progress
    DROP CONSTRAINT IF EXISTS ck_user_checklist_progress_status;

ALTER TABLE user_checklist_progress
    ADD CONSTRAINT ck_user_checklist_progress_status
    CHECK (status IN ('unavailable', 'active', 'completed'));

ALTER TABLE checklist_items
    DROP CONSTRAINT IF EXISTS fk_checklist_items_stage;

ALTER TABLE checklist_items
    DROP COLUMN IF EXISTS stage_id,
    DROP COLUMN IF EXISTS expected_result,
    DROP COLUMN IF EXISTS requires_review;

DROP TABLE IF EXISTS checklist_stages;
