-- +goose Up
CREATE TABLE recurrence_series (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL REFERENCES users(id),
 payload jsonb NOT NULL,
 ends_before date,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_at timestamptz NOT NULL,
 UNIQUE(owner_id,id)
);
ALTER TABLE calendar_events ADD COLUMN series_id uuid;
ALTER TABLE calendar_events ADD COLUMN occurrence_key date;
ALTER TABLE calendar_events ADD FOREIGN KEY(owner_id,series_id) REFERENCES recurrence_series(owner_id,id);
ALTER TABLE calendar_events ADD CONSTRAINT calendar_occurrence_identity CHECK((series_id IS NULL)=(occurrence_key IS NULL));
CREATE UNIQUE INDEX calendar_occurrence_unique ON calendar_events(series_id,occurrence_key) WHERE series_id IS NOT NULL;
CREATE TABLE recurrence_exclusions (
 owner_id uuid NOT NULL,
 series_id uuid NOT NULL,
 occurrence_key date NOT NULL,
 reason text NOT NULL,
 PRIMARY KEY(series_id,occurrence_key),
 FOREIGN KEY(owner_id,series_id) REFERENCES recurrence_series(owner_id,id)
);
-- +goose Down
DROP TABLE recurrence_exclusions;
ALTER TABLE calendar_events DROP CONSTRAINT calendar_occurrence_identity;
ALTER TABLE calendar_events DROP COLUMN occurrence_key;
ALTER TABLE calendar_events DROP COLUMN series_id;
DROP TABLE recurrence_series;
