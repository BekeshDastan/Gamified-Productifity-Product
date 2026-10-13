-- +goose Up
ALTER TABLE habits ADD COLUMN materialized_through date;
CREATE TABLE habit_periods (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL,habit_id uuid NOT NULL,period_key text NOT NULL,
 starts_at timestamptz NOT NULL,ends_at timestamptz NOT NULL CHECK(ends_at>starts_at),timezone text NOT NULL,schedule_version bigint NOT NULL,
 target numeric(15,3) NOT NULL CHECK(target>0),excluded boolean NOT NULL DEFAULT false,emergency_reason text NOT NULL DEFAULT '',
 UNIQUE(owner_id,id),UNIQUE(owner_id,habit_id,period_key),
 FOREIGN KEY(owner_id,habit_id,schedule_version) REFERENCES habit_schedules(owner_id,habit_id,version)
);
CREATE INDEX habit_periods_history ON habit_periods(owner_id,habit_id,starts_at);
-- +goose Down
DROP TABLE habit_periods;
ALTER TABLE habits DROP COLUMN materialized_through;
