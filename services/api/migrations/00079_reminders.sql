-- +goose Up
CREATE TABLE reminders(
 owner_id uuid NOT NULL REFERENCES users(id),id uuid NOT NULL DEFAULT gen_random_uuid(),
 source_kind text NOT NULL CHECK(source_kind IN('task','habit_period')),source_id uuid NOT NULL,parent_id uuid,
 source_revision bigint NOT NULL CHECK(source_revision>0),source_due_at timestamptz NOT NULL,
 next_run_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),state text NOT NULL CHECK(state IN('scheduled','fired','cancelled')),
 urgent_override boolean NOT NULL DEFAULT false,snoozed_until timestamptz,notification_id uuid,
 source_url text NOT NULL,created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,id),UNIQUE(owner_id,source_kind,source_id,source_revision,source_due_at),
 FOREIGN KEY(owner_id,notification_id) REFERENCES notifications(owner_id,id)
);
CREATE INDEX reminders_owner_pending ON reminders(owner_id,next_run_at,id) WHERE state='scheduled';
ALTER TABLE notifications ADD CONSTRAINT notifications_reminder_owner FOREIGN KEY(owner_id,reminder_id) REFERENCES reminders(owner_id,id);
-- +goose Down
ALTER TABLE notifications DROP CONSTRAINT notifications_reminder_owner;
DROP TABLE reminders;
