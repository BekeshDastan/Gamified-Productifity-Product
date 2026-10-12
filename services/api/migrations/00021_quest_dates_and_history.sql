-- +goose Up
-- Date-only plans and actual moments have separate PostgreSQL types.
ALTER TABLE quests ADD COLUMN planned_date date;
ALTER TABLE quests ADD COLUMN deadline timestamptz;
UPDATE quests SET planned_date=(payload->>'planned_date')::date,deadline=(payload->>'deadline')::timestamptz;
ALTER TABLE quests ADD CONSTRAINT quests_planned_date_sync CHECK((payload->>'planned_date')::date IS NOT DISTINCT FROM planned_date);
ALTER TABLE quests ADD CONSTRAINT quests_deadline_sync CHECK((payload->>'deadline')::timestamptz IS NOT DISTINCT FROM deadline);
CREATE INDEX quests_planned ON quests(owner_id,planned_date,id);
CREATE INDEX quests_deadline ON quests(owner_id,deadline) WHERE archived_at IS NULL AND status NOT IN('completed','failed','skipped');
ALTER TABLE task_events ADD COLUMN details jsonb NOT NULL DEFAULT '{}';
-- +goose Down
ALTER TABLE task_events DROP COLUMN details;
ALTER TABLE quests DROP CONSTRAINT quests_planned_date_sync;
ALTER TABLE quests DROP CONSTRAINT quests_deadline_sync;
ALTER TABLE quests DROP COLUMN planned_date;
ALTER TABLE quests DROP COLUMN deadline;
