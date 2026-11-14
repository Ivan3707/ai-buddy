-- +goose Up
-- Comment under a thumbs-down: what was wrong with the answer. Only an addition, existing relations are not changed.
ALTER TABLE public.chat_messages ADD COLUMN feedback_comment text;

-- +goose Down
ALTER TABLE public.chat_messages DROP COLUMN feedback_comment;
