-- +goose Up
-- P2-07: limits of task comments that were enforced only by TaskCommentService (internal/service/task_comments.go).
-- Values mirror MaxCommentChars, MaxFileBytes and maxFilenameLength. Per-comment limits that span several rows
-- (at most 5 files, at most 10 MB in total, "text or at least one file") stay in the service.
ALTER TABLE public.task_comments
    ADD CONSTRAINT task_comments_body_length_check CHECK (char_length(body) <= 2000);

ALTER TABLE public.task_attachments
    ADD CONSTRAINT task_attachments_size_check CHECK (size_bytes > 0 AND size_bytes <= 5242880),
    ADD CONSTRAINT task_attachments_data_size_check CHECK (octet_length(data) = size_bytes),
    ADD CONSTRAINT task_attachments_filename_check CHECK (char_length(filename) BETWEEN 1 AND 120),
    ADD CONSTRAINT task_attachments_content_type_check CHECK (content_type <> '');

-- +goose Down
ALTER TABLE public.task_attachments
    DROP CONSTRAINT task_attachments_content_type_check,
    DROP CONSTRAINT task_attachments_filename_check,
    DROP CONSTRAINT task_attachments_data_size_check,
    DROP CONSTRAINT task_attachments_size_check;

ALTER TABLE public.task_comments
    DROP CONSTRAINT task_comments_body_length_check;
