-- +goose Up
-- AI assistant: conversations, messages, confirmed actions, mock corporate requests and questions escalated to the buddy.
-- Only additions: existing tables and relations are not changed.

CREATE TABLE public.chat_conversations (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE INDEX idx_chat_conversations_user ON public.chat_conversations (user_id, updated_at DESC);

CREATE TABLE public.assistant_actions (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    type text NOT NULL CHECK (type IN ('leave_request', 'access_request', 'mentor_question')),
    params jsonb NOT NULL,
    summary text NOT NULL,
    status text DEFAULT 'pending' NOT NULL CHECK (status IN ('pending', 'executed', 'cancelled', 'failed')),
    error text,
    result jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.chat_messages (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    conversation_id uuid NOT NULL REFERENCES public.chat_conversations(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('user', 'assistant')),
    content text NOT NULL,
    sources jsonb,
    action_id uuid REFERENCES public.assistant_actions(id) ON DELETE SET NULL,
    finish_reason text,
    feedback text CHECK (feedback IN ('up', 'down')),
    created_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE INDEX idx_chat_messages_conversation ON public.chat_messages (conversation_id, created_at);

CREATE TABLE public.service_requests (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    action_id uuid UNIQUE REFERENCES public.assistant_actions(id) ON DELETE SET NULL,
    number text NOT NULL UNIQUE,
    type text NOT NULL CHECK (type IN ('leave_request', 'access_request')),
    status text DEFAULT 'pending_approval' NOT NULL CHECK (status IN ('pending_approval', 'approved', 'rejected')),
    approver text,
    params jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE INDEX idx_service_requests_user ON public.service_requests (user_id, created_at DESC);
CREATE SEQUENCE public.service_request_number_seq;

CREATE TABLE public.mentor_questions (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    intern_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    buddy_id uuid REFERENCES public.users(id) ON DELETE SET NULL,
    action_id uuid UNIQUE REFERENCES public.assistant_actions(id) ON DELETE SET NULL,
    question text NOT NULL,
    context text DEFAULT '' NOT NULL,
    status text DEFAULT 'open' NOT NULL CHECK (status IN ('open', 'answered')),
    escalation_level text DEFAULT 'mentor' NOT NULL CHECK (escalation_level IN ('mentor', 'lead', 'hrbp')),
    answer text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    answered_at timestamp with time zone
);
CREATE INDEX idx_mentor_questions_buddy ON public.mentor_questions (buddy_id, status, created_at DESC);

-- +goose Down
DROP TABLE public.mentor_questions;
DROP SEQUENCE public.service_request_number_seq;
DROP TABLE public.service_requests;
DROP TABLE public.chat_messages;
DROP TABLE public.assistant_actions;
DROP TABLE public.chat_conversations;
