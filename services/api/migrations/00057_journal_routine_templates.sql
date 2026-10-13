-- +goose Up
CREATE TABLE journal_routine_templates (
 owner_id uuid NOT NULL REFERENCES users(id), kind text NOT NULL CHECK(kind IN('morning','evening')),
 steps jsonb NOT NULL CHECK(jsonb_typeof(steps)='array' AND jsonb_array_length(steps)<=20),
 version bigint NOT NULL DEFAULT 1, updated_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,kind)
);
ALTER TABLE routine_run_events ADD COLUMN id bigint GENERATED ALWAYS AS IDENTITY;
CREATE UNIQUE INDEX routine_run_events_cursor ON routine_run_events(owner_id,id DESC);
-- +goose Down
DROP INDEX routine_run_events_cursor;
ALTER TABLE routine_run_events DROP COLUMN id;
DROP TABLE journal_routine_templates;
