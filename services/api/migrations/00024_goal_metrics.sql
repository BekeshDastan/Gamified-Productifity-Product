-- +goose Up
ALTER TABLE goals ADD COLUMN current numeric NOT NULL DEFAULT 0;
UPDATE goals SET current=(payload->>'start')::numeric WHERE payload->>'progress_type'='metric';
CREATE TABLE goal_metrics (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL,
 goal_id uuid NOT NULL,
 start_value numeric(13,3) NOT NULL,
 target numeric(13,3) NOT NULL CHECK(target>start_value),
 unit text NOT NULL CHECK(char_length(unit) BETWEEN 1 AND 40),
 UNIQUE(owner_id,goal_id),
 UNIQUE(owner_id,goal_id,id),
 FOREIGN KEY(owner_id,goal_id) REFERENCES goals(owner_id,id)
);
INSERT INTO goal_metrics(owner_id,goal_id,start_value,target,unit)
SELECT owner_id,id,(payload->>'start')::numeric,(payload->>'target')::numeric,payload->>'unit' FROM goals WHERE payload->>'progress_type'='metric';
CREATE TABLE goal_metric_entries (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 owner_id uuid NOT NULL,
 goal_id uuid NOT NULL,
 metric_id uuid NOT NULL,
 value numeric(13,3) NOT NULL CHECK(value>=0),
 unit text NOT NULL,
 occurred_on date NOT NULL,
 note text NOT NULL,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL,
 UNIQUE(owner_id,id),
 FOREIGN KEY(owner_id,goal_id,metric_id) REFERENCES goal_metrics(owner_id,goal_id,id)
);
CREATE INDEX goal_entries_page ON goal_metric_entries(owner_id,goal_id,id DESC);
CREATE TABLE goal_metric_revisions (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 owner_id uuid NOT NULL,
 entry_id bigint NOT NULL,
 version bigint NOT NULL,
 value numeric(13,3) NOT NULL,
 unit text NOT NULL,
 occurred_on date NOT NULL,
 note text NOT NULL,
 reason text NOT NULL,
 created_at timestamptz NOT NULL,
 UNIQUE(entry_id,version),
 FOREIGN KEY(owner_id,entry_id) REFERENCES goal_metric_entries(owner_id,id)
);
-- +goose Down
DROP TABLE goal_metric_revisions;
DROP TABLE goal_metric_entries;
DROP TABLE goal_metrics;
ALTER TABLE goals DROP COLUMN current;
