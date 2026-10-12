-- +goose Up
CREATE TABLE task_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 owner_id uuid NOT NULL,
 quest_id uuid NOT NULL,
 kind text NOT NULL,
 reason text NOT NULL DEFAULT '',
 before_state jsonb,
 after_state jsonb NOT NULL,
 quest_version bigint NOT NULL,
 created_at timestamptz NOT NULL,
 UNIQUE(quest_id,quest_version),
 FOREIGN KEY(owner_id,quest_id) REFERENCES quests(owner_id,id)
);
CREATE INDEX task_events_owner_quest ON task_events(owner_id,quest_id,id DESC);
-- +goose Down
DROP TABLE task_events;
