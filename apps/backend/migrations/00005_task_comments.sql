-- +goose Up
-- Comments of an intern and their mentor / teamlead under one task (one thread per progress row), with small attachments.
CREATE TABLE public.task_comments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    progress_id uuid NOT NULL,
    author_id uuid NOT NULL,
    author_role character varying(20) NOT NULL,
    body text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT task_comments_pkey PRIMARY KEY (id),
    CONSTRAINT task_comments_progress_fkey FOREIGN KEY (progress_id) REFERENCES public.user_checklist_progress(id) ON DELETE CASCADE,
    CONSTRAINT task_comments_author_fkey FOREIGN KEY (author_id) REFERENCES public.users(id),
    CONSTRAINT task_comments_role_check CHECK (author_role IN ('intern', 'buddy', 'teamlead'))
);
CREATE INDEX task_comments_progress_idx ON public.task_comments (progress_id, created_at);

CREATE TABLE public.task_attachments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    comment_id uuid NOT NULL,
    filename text NOT NULL,
    content_type text NOT NULL,
    size_bytes integer NOT NULL,
    data bytea NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT task_attachments_pkey PRIMARY KEY (id),
    CONSTRAINT task_attachments_comment_fkey FOREIGN KEY (comment_id) REFERENCES public.task_comments(id) ON DELETE CASCADE
);
CREATE INDEX task_attachments_comment_idx ON public.task_attachments (comment_id);

-- +goose Down
DROP TABLE public.task_attachments;
DROP TABLE public.task_comments;
