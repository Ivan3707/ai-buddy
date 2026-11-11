-- +goose Up

-- Do not silently normalize inconsistent progress data. If migration 00002
-- left or admitted invalid rows, stop and report every affected progress ID.
-- +goose StatementBegin
DO $$
DECLARE
    inconsistent_count BIGINT;
    inconsistent_ids UUID[];
BEGIN
    SELECT
        COUNT(*),
        ARRAY_AGG(id ORDER BY id)
    INTO
        inconsistent_count,
        inconsistent_ids
    FROM user_checklist_progress
    WHERE NOT (
        (
            status IN ('completed', 'verified')
            AND completed_at IS NOT NULL
        )
        OR
        (
            status IN ('unavailable', 'active')
            AND completed_at IS NULL
        )
    );

    IF inconsistent_count > 0 THEN
        RAISE EXCEPTION
            'cannot restore progress completed_at constraint: % inconsistent rows, ids: %',
            inconsistent_count,
            inconsistent_ids;
    END IF;
END
$$;
-- +goose StatementEnd

DROP INDEX uq_progress_one_active_per_checklist;

ALTER TABLE user_checklist_progress
    ADD CONSTRAINT chk_progress_completed_at_consistency
    CHECK (
        (
            status IN ('completed', 'verified')
            AND completed_at IS NOT NULL
        )
        OR
        (
            status IN ('unavailable', 'active')
            AND completed_at IS NULL
        )
    );

ALTER TABLE checklist_items
    ADD COLUMN estimated_minutes INTEGER,
    ADD CONSTRAINT chk_checklist_items_estimated_minutes
    CHECK (estimated_minutes > 0);

ALTER TABLE users
    ADD COLUMN last_login_at TIMESTAMPTZ;

CREATE TABLE checklist_item_submissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    progress_id UUID NOT NULL,
    attempt_number INTEGER NOT NULL,
    result_text TEXT,
    result_url TEXT,
    status VARCHAR(20) NOT NULL,
    submitted_at TIMESTAMPTZ NOT NULL,
    reviewed_at TIMESTAMPTZ,
    reviewed_by UUID,
    review_comment TEXT,

    CONSTRAINT fk_checklist_item_submissions_progress
        FOREIGN KEY (progress_id)
        REFERENCES user_checklist_progress (id),

    CONSTRAINT fk_checklist_item_submissions_reviewer
        FOREIGN KEY (reviewed_by)
        REFERENCES users (id),

    CONSTRAINT uq_checklist_item_submissions_attempt
        UNIQUE (progress_id, attempt_number),

    CONSTRAINT chk_checklist_item_submissions_attempt_number
        CHECK (attempt_number > 0),

    CONSTRAINT chk_checklist_item_submissions_status
        CHECK (status IN ('pending', 'approved', 'rejected')),

    CONSTRAINT chk_checklist_item_submissions_result
        CHECK (
            NULLIF(BTRIM(result_text), '') IS NOT NULL
            OR NULLIF(BTRIM(result_url), '') IS NOT NULL
        ),

    CONSTRAINT chk_checklist_item_submissions_review
        CHECK (
            (
                status = 'pending'
                AND reviewed_at IS NULL
                AND reviewed_by IS NULL
                AND review_comment IS NULL
            )
            OR
            (
                status = 'approved'
                AND reviewed_at IS NOT NULL
                AND reviewed_by IS NOT NULL
            )
            OR
            (
                status = 'rejected'
                AND reviewed_at IS NOT NULL
                AND reviewed_by IS NOT NULL
                AND NULLIF(BTRIM(review_comment), '') IS NOT NULL
            )
        )
);

CREATE UNIQUE INDEX uq_checklist_item_submissions_pending
    ON checklist_item_submissions (progress_id)
    WHERE status = 'pending';

CREATE INDEX ix_checklist_item_submissions_status_submitted_at
    ON checklist_item_submissions (status, submitted_at);


-- +goose Down

-- Creating this index intentionally fails if the Up migration has allowed
-- multiple active items for the same user and checklist. No data is changed
-- to make the rollback succeed.
CREATE UNIQUE INDEX uq_progress_one_active_per_checklist
    ON user_checklist_progress (user_id, checklist_id)
    WHERE status = 'active';

DROP TABLE checklist_item_submissions;

ALTER TABLE users
    DROP COLUMN last_login_at;

ALTER TABLE checklist_items
    DROP CONSTRAINT chk_checklist_items_estimated_minutes,
    DROP COLUMN estimated_minutes;

ALTER TABLE user_checklist_progress
    DROP CONSTRAINT chk_progress_completed_at_consistency;
