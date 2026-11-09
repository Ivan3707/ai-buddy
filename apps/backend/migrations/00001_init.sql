-- +goose Up

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL,
    profession VARCHAR(20),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_users_role
        CHECK (role IN ('intern', 'teamlead')),

    CONSTRAINT chk_users_profession
        CHECK (
            profession IS NULL
            OR profession IN ('backend', 'frontend', 'ml')
        ),

    CONSTRAINT chk_intern_profession
        CHECK (role <> 'intern' OR profession IS NOT NULL)
);

CREATE UNIQUE INDEX uq_users_email_lower
    ON users (LOWER(email));


CREATE TABLE checklists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(20) NOT NULL,
    profession VARCHAR(20),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_checklists_type
        CHECK (type IN ('common', 'professional')),

    CONSTRAINT chk_checklists_profession
        CHECK (
            (type = 'common' AND profession IS NULL)
            OR
            (type = 'professional'
                AND profession IN ('backend', 'frontend', 'ml'))
        )
);

-- В системе может быть только один общий чек-лист.
CREATE UNIQUE INDEX uq_checklists_common
    ON checklists (type)
    WHERE type = 'common';

-- Для каждого профессионального направления может быть
-- только один профессиональный чек-лист.
CREATE UNIQUE INDEX uq_checklists_profession
    ON checklists (profession)
    WHERE type = 'professional';


CREATE TABLE checklist_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    checklist_id UUID NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_checklist_items_checklist
        FOREIGN KEY (checklist_id)
        REFERENCES checklists (id)
        ON DELETE CASCADE,

    CONSTRAINT uq_checklist_items_position
        UNIQUE (checklist_id, position),

    -- Нужно для составного внешнего ключа в user_checklist_progress.
    CONSTRAINT uq_checklist_items_id_checklist
        UNIQUE (id, checklist_id)
);


CREATE TABLE user_checklist_progress (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    checklist_id UUID NOT NULL,
    checklist_item_id UUID NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'unavailable',
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_progress_user
        FOREIGN KEY (user_id)
        REFERENCES users (id)
        ON DELETE CASCADE,

    -- Проверяем, что задание действительно принадлежит
    -- указанному чек-листу.
    CONSTRAINT fk_progress_checklist_item
        FOREIGN KEY (checklist_item_id, checklist_id)
        REFERENCES checklist_items (id, checklist_id)
        ON DELETE CASCADE,

    CONSTRAINT uq_user_checklist_item
        UNIQUE (user_id, checklist_item_id),

    CONSTRAINT chk_progress_status
        CHECK (status IN ('unavailable', 'active', 'completed')),

    CONSTRAINT chk_progress_completed_at
        CHECK (
            (status = 'completed' AND completed_at IS NOT NULL)
            OR
            (status <> 'completed' AND completed_at IS NULL)
        )
);

-- Один стажёр может иметь не более одного активного задания
-- в рамках одного чек-листа.
CREATE UNIQUE INDEX uq_progress_one_active_per_checklist
    ON user_checklist_progress (user_id, checklist_id)
    WHERE status = 'active';


-- +goose Down

DROP TABLE IF EXISTS user_checklist_progress;
DROP TABLE IF EXISTS checklist_items;
DROP TABLE IF EXISTS checklists;
DROP TABLE IF EXISTS users;
