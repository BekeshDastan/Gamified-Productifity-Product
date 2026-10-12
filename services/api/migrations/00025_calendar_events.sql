-- +goose Up
CREATE TABLE calendar_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL REFERENCES users(id),
 quest_id uuid,
 payload jsonb NOT NULL,
 all_day boolean NOT NULL,
 start_date date,
 end_date date,
 starts_at timestamptz,
 ends_at timestamptz,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 cancelled boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 UNIQUE(owner_id,id),
 FOREIGN KEY(owner_id,quest_id) REFERENCES quests(owner_id,id),
 CHECK((all_day AND start_date IS NOT NULL AND end_date>start_date AND starts_at IS NULL AND ends_at IS NULL)
 OR (NOT all_day AND start_date IS NULL AND end_date IS NULL AND starts_at IS NOT NULL AND ends_at>starts_at))
);
CREATE INDEX calendar_dates ON calendar_events(owner_id,start_date,end_date) WHERE all_day;
CREATE INDEX calendar_instants ON calendar_events(owner_id,starts_at,ends_at) WHERE NOT all_day;
CREATE INDEX calendar_quest ON calendar_events(owner_id,quest_id);
CREATE TABLE calendar_event_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 owner_id uuid NOT NULL,
 event_id uuid NOT NULL,
 before_state jsonb,
 after_state jsonb NOT NULL,
 reason text NOT NULL,
 created_at timestamptz NOT NULL,
 FOREIGN KEY(owner_id,event_id) REFERENCES calendar_events(owner_id,id)
);
CREATE INDEX calendar_history_event ON calendar_event_history(owner_id,event_id,id DESC);
-- +goose Down
DROP TABLE calendar_event_history;
DROP TABLE calendar_events;
