-- +goose Up

CREATE TABLE public.auth_sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash bytea NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.calendar_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    title text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    starts_at timestamp with time zone NOT NULL,
    ends_at timestamp with time zone NOT NULL,
    CONSTRAINT calendar_events_check CHECK ((ends_at > starts_at))
);

CREATE TABLE public.checklist_feedback (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    checklist_id uuid NOT NULL,
    rating integer NOT NULL,
    comment text DEFAULT ''::text NOT NULL,
    delivery_status text DEFAULT 'pending'::text NOT NULL,
    delivery_error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    delivered_at timestamp with time zone,
    CONSTRAINT checklist_feedback_delivery_status_check CHECK ((delivery_status = ANY (ARRAY['pending'::text, 'sent'::text, 'failed'::text]))),
    CONSTRAINT checklist_feedback_rating_check CHECK (((rating >= 1) AND (rating <= 5)))
);

CREATE TABLE public.checklist_item_submissions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    progress_id uuid NOT NULL,
    attempt_number integer NOT NULL,
    result_text text,
    result_url text,
    status character varying(20) NOT NULL,
    submitted_at timestamp with time zone NOT NULL,
    reviewed_at timestamp with time zone,
    reviewed_by uuid,
    review_comment text,
    CONSTRAINT chk_checklist_item_submissions_attempt_number CHECK ((attempt_number > 0)),
    CONSTRAINT chk_checklist_item_submissions_result CHECK (((NULLIF(btrim(result_text), ''::text) IS NOT NULL) OR (NULLIF(btrim(result_url), ''::text) IS NOT NULL))),
    CONSTRAINT chk_checklist_item_submissions_review CHECK (((((status)::text = 'pending'::text) AND (reviewed_at IS NULL) AND (reviewed_by IS NULL) AND (review_comment IS NULL)) OR (((status)::text = 'approved'::text) AND (reviewed_at IS NOT NULL) AND (reviewed_by IS NOT NULL)) OR (((status)::text = 'rejected'::text) AND (reviewed_at IS NOT NULL) AND (reviewed_by IS NOT NULL) AND (NULLIF(btrim(review_comment), ''::text) IS NOT NULL)))),
    CONSTRAINT chk_checklist_item_submissions_status CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'approved'::character varying, 'rejected'::character varying])::text[])))
);

CREATE TABLE public.checklist_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    checklist_id uuid NOT NULL,
    title character varying(255) NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    "position" integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    stage_id uuid NOT NULL,
    expected_result text,
    requires_review boolean DEFAULT false NOT NULL,
    estimated_minutes integer,
    assigned_user_id uuid,
    due_at timestamp with time zone,
    CONSTRAINT chk_checklist_items_estimated_minutes CHECK ((estimated_minutes > 0))
);

CREATE TABLE public.checklist_stages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    checklist_id uuid NOT NULL,
    title character varying(255) NOT NULL,
    week_start integer NOT NULL,
    week_end integer NOT NULL,
    goal text DEFAULT ''::text NOT NULL,
    expected_result text DEFAULT ''::text NOT NULL,
    "position" integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT checklist_stages_check CHECK ((week_end >= week_start)),
    CONSTRAINT checklist_stages_position_check CHECK (("position" >= 1)),
    CONSTRAINT checklist_stages_week_start_check CHECK ((week_start >= 1))
);

CREATE TABLE public.checklists (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(255) NOT NULL,
    type character varying(20) NOT NULL,
    profession character varying(20),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_checklists_profession CHECK (((((type)::text = 'common'::text) AND (profession IS NULL)) OR (((type)::text = 'professional'::text) AND ((profession)::text = ANY ((ARRAY['backend'::character varying, 'frontend'::character varying, 'ml'::character varying])::text[]))))),
    CONSTRAINT chk_checklists_type CHECK (((type)::text = ANY ((ARRAY['common'::character varying, 'professional'::character varying])::text[])))
);

CREATE TABLE public.corporate_access_requests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    tool text NOT NULL,
    idempotency_key text NOT NULL,
    ticket_number text NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT corporate_access_requests_status_check CHECK ((status = ANY (ARRAY['open'::text, 'approved'::text, 'denied'::text])))
);

CREATE TABLE public.mock_analytics_feedback (
    feedback_id uuid NOT NULL,
    corporate_id text NOT NULL,
    checklist_id uuid NOT NULL,
    checklist_type text NOT NULL,
    rating integer NOT NULL,
    comment text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    received_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.notes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    title text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.task_hints (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    progress_id uuid NOT NULL,
    text text,
    attempts integer DEFAULT 0 NOT NULL,
    next_attempt_at timestamp with time zone DEFAULT now() NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    read_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT task_hints_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'processing'::text, 'ready'::text])))
);

CREATE TABLE public.user_checklist_progress (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    checklist_id uuid NOT NULL,
    checklist_item_id uuid NOT NULL,
    status character varying(20) DEFAULT 'unavailable'::character varying NOT NULL,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    "position" integer NOT NULL,
    activated_at timestamp with time zone,
    CONSTRAINT chk_progress_completed_at_consistency CHECK (((((status)::text = ANY ((ARRAY['completed'::character varying, 'verified'::character varying])::text[])) AND (completed_at IS NOT NULL)) OR (((status)::text = ANY ((ARRAY['unavailable'::character varying, 'active'::character varying])::text[])) AND (completed_at IS NULL)))),
    CONSTRAINT ck_user_checklist_progress_status CHECK (((status)::text = ANY ((ARRAY['unavailable'::character varying, 'active'::character varying, 'completed'::character varying, 'verified'::character varying])::text[])))
);

CREATE TABLE public.users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    email character varying(255) NOT NULL,
    password_hash character varying(255) NOT NULL,
    role character varying(20) NOT NULL,
    profession character varying(20),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    buddy_id uuid,
    last_login_at timestamp with time zone,
    first_name text,
    last_name text,
    telegram_username text,
    department text,
    team text,
    teamlead_id uuid,
    birth_date date,
    employed_since date,
    corporate_id text,
    CONSTRAINT chk_intern_profession CHECK ((((role)::text <> 'intern'::text) OR (profession IS NOT NULL))),
    CONSTRAINT chk_users_profession CHECK (((profession IS NULL) OR ((profession)::text = ANY ((ARRAY['backend'::character varying, 'frontend'::character varying, 'ml'::character varying])::text[])))),
    CONSTRAINT ck_users_role CHECK (((role)::text = ANY ((ARRAY['intern'::character varying, 'buddy'::character varying, 'teamlead'::character varying])::text[])))
);

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY public.calendar_events
    ADD CONSTRAINT calendar_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.checklist_feedback
    ADD CONSTRAINT checklist_feedback_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.checklist_feedback
    ADD CONSTRAINT checklist_feedback_user_id_checklist_id_key UNIQUE (user_id, checklist_id);

ALTER TABLE ONLY public.checklist_item_submissions
    ADD CONSTRAINT checklist_item_submissions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.checklist_items
    ADD CONSTRAINT checklist_items_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.checklist_stages
    ADD CONSTRAINT checklist_stages_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.checklists
    ADD CONSTRAINT checklists_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.corporate_access_requests
    ADD CONSTRAINT corporate_access_requests_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.corporate_access_requests
    ADD CONSTRAINT corporate_access_requests_ticket_number_key UNIQUE (ticket_number);

ALTER TABLE ONLY public.corporate_access_requests
    ADD CONSTRAINT corporate_access_requests_user_id_idempotency_key_key UNIQUE (user_id, idempotency_key);

ALTER TABLE ONLY public.mock_analytics_feedback
    ADD CONSTRAINT mock_analytics_feedback_pkey PRIMARY KEY (feedback_id);

ALTER TABLE ONLY public.notes
    ADD CONSTRAINT notes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.task_hints
    ADD CONSTRAINT task_hints_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.task_hints
    ADD CONSTRAINT task_hints_progress_id_key UNIQUE (progress_id);

ALTER TABLE ONLY public.checklist_item_submissions
    ADD CONSTRAINT uq_checklist_item_submissions_attempt UNIQUE (progress_id, attempt_number);

ALTER TABLE ONLY public.checklist_items
    ADD CONSTRAINT uq_checklist_items_id_checklist UNIQUE (id, checklist_id);

ALTER TABLE ONLY public.checklist_items
    ADD CONSTRAINT uq_checklist_items_position UNIQUE (checklist_id, "position");

ALTER TABLE ONLY public.checklist_stages
    ADD CONSTRAINT uq_checklist_stages_id_checklist UNIQUE (id, checklist_id);

ALTER TABLE ONLY public.checklist_stages
    ADD CONSTRAINT uq_checklist_stages_position UNIQUE (checklist_id, "position");

ALTER TABLE ONLY public.user_checklist_progress
    ADD CONSTRAINT uq_user_checklist_item UNIQUE (user_id, checklist_item_id);

ALTER TABLE ONLY public.user_checklist_progress
    ADD CONSTRAINT user_checklist_progress_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

CREATE INDEX auth_sessions_user_id_idx ON public.auth_sessions USING btree (user_id);

CREATE INDEX calendar_events_owner_idx ON public.calendar_events USING btree (user_id, starts_at);

CREATE INDEX ix_checklist_feedback_delivery ON public.checklist_feedback USING btree (delivery_status, created_at) WHERE (delivery_status <> 'sent'::text);

CREATE INDEX ix_checklist_item_submissions_status_submitted_at ON public.checklist_item_submissions USING btree (status, submitted_at);

CREATE INDEX ix_corporate_access_requests_user ON public.corporate_access_requests USING btree (user_id, created_at);

CREATE INDEX ix_users_buddy_id ON public.users USING btree (buddy_id);

CREATE INDEX ix_users_teamlead_id ON public.users USING btree (teamlead_id);

CREATE INDEX notes_owner_idx ON public.notes USING btree (user_id, updated_at);

CREATE INDEX progress_user_checklist_position_idx ON public.user_checklist_progress USING btree (user_id, checklist_id, "position");

CREATE INDEX task_hints_due_idx ON public.task_hints USING btree (status, next_attempt_at);

CREATE UNIQUE INDEX uq_checklist_item_submissions_pending ON public.checklist_item_submissions USING btree (progress_id) WHERE ((status)::text = 'pending'::text);

CREATE UNIQUE INDEX uq_checklists_common ON public.checklists USING btree (type) WHERE ((type)::text = 'common'::text);

CREATE UNIQUE INDEX uq_checklists_profession ON public.checklists USING btree (profession) WHERE ((type)::text = 'professional'::text);

CREATE UNIQUE INDEX uq_users_corporate_id ON public.users USING btree (corporate_id) WHERE (corporate_id IS NOT NULL);

CREATE UNIQUE INDEX uq_users_email_lower ON public.users USING btree (lower((email)::text));

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.calendar_events
    ADD CONSTRAINT calendar_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.checklist_feedback
    ADD CONSTRAINT checklist_feedback_checklist_id_fkey FOREIGN KEY (checklist_id) REFERENCES public.checklists(id);

ALTER TABLE ONLY public.checklist_feedback
    ADD CONSTRAINT checklist_feedback_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);

ALTER TABLE ONLY public.checklist_items
    ADD CONSTRAINT checklist_items_assigned_user_id_fkey FOREIGN KEY (assigned_user_id) REFERENCES public.users(id);

ALTER TABLE ONLY public.checklist_stages
    ADD CONSTRAINT checklist_stages_checklist_id_fkey FOREIGN KEY (checklist_id) REFERENCES public.checklists(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.corporate_access_requests
    ADD CONSTRAINT corporate_access_requests_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.checklist_item_submissions
    ADD CONSTRAINT fk_checklist_item_submissions_progress FOREIGN KEY (progress_id) REFERENCES public.user_checklist_progress(id);

ALTER TABLE ONLY public.checklist_item_submissions
    ADD CONSTRAINT fk_checklist_item_submissions_reviewer FOREIGN KEY (reviewed_by) REFERENCES public.users(id);

ALTER TABLE ONLY public.checklist_items
    ADD CONSTRAINT fk_checklist_items_checklist FOREIGN KEY (checklist_id) REFERENCES public.checklists(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.checklist_items
    ADD CONSTRAINT fk_checklist_items_stage FOREIGN KEY (stage_id, checklist_id) REFERENCES public.checklist_stages(id, checklist_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_checklist_progress
    ADD CONSTRAINT fk_progress_checklist_item FOREIGN KEY (checklist_item_id, checklist_id) REFERENCES public.checklist_items(id, checklist_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_checklist_progress
    ADD CONSTRAINT fk_progress_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.notes
    ADD CONSTRAINT notes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.task_hints
    ADD CONSTRAINT task_hints_progress_id_fkey FOREIGN KEY (progress_id) REFERENCES public.user_checklist_progress(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_buddy_id_fkey FOREIGN KEY (buddy_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_teamlead_id_fkey FOREIGN KEY (teamlead_id) REFERENCES public.users(id) ON DELETE SET NULL;

-- +goose Down

DROP TABLE IF EXISTS public.users CASCADE;
DROP TABLE IF EXISTS public.user_checklist_progress CASCADE;
DROP TABLE IF EXISTS public.task_hints CASCADE;
DROP TABLE IF EXISTS public.notes CASCADE;
DROP TABLE IF EXISTS public.mock_analytics_feedback CASCADE;
DROP TABLE IF EXISTS public.corporate_access_requests CASCADE;
DROP TABLE IF EXISTS public.checklists CASCADE;
DROP TABLE IF EXISTS public.checklist_stages CASCADE;
DROP TABLE IF EXISTS public.checklist_items CASCADE;
DROP TABLE IF EXISTS public.checklist_item_submissions CASCADE;
DROP TABLE IF EXISTS public.checklist_feedback CASCADE;
DROP TABLE IF EXISTS public.calendar_events CASCADE;
DROP TABLE IF EXISTS public.auth_sessions CASCADE;
