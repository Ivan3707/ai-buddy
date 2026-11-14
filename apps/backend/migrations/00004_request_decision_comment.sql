-- +goose Up
-- The teamlead's comment on a decision (approve / reject) of a mock corporate request. Only an addition.
ALTER TABLE public.service_requests ADD COLUMN decision_comment text;

-- +goose Down
ALTER TABLE public.service_requests DROP COLUMN decision_comment;
