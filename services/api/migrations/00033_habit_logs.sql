-- +goose Up
ALTER TABLE habit_periods ADD CONSTRAINT habit_periods_owner_habit_id UNIQUE(owner_id,habit_id,id);
CREATE TABLE habit_logs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner_id uuid NOT NULL,habit_id uuid NOT NULL,period_id uuid NOT NULL,
 amount numeric(15,3) NOT NULL CHECK(amount>=0),original_amount numeric(15,3) NOT NULL CHECK(original_amount>0),
 occurred_at timestamptz NOT NULL,accepted_at timestamptz NOT NULL,backfilled boolean NOT NULL,
 UNIQUE(owner_id,id),FOREIGN KEY(owner_id,habit_id,period_id) REFERENCES habit_periods(owner_id,habit_id,id),FOREIGN KEY(owner_id,habit_id) REFERENCES habits(owner_id,id)
);
CREATE INDEX habit_logs_period ON habit_logs(owner_id,period_id);
CREATE TABLE habit_log_corrections (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,owner_id uuid NOT NULL,log_id uuid NOT NULL,
 before_amount numeric(15,3) NOT NULL CHECK(before_amount>=0),after_amount numeric(15,3) NOT NULL CHECK(after_amount>=0),reason text NOT NULL CHECK(length(trim(reason))>0),created_at timestamptz NOT NULL,
 FOREIGN KEY(owner_id,log_id) REFERENCES habit_logs(owner_id,id)
);
-- +goose Down
DROP TABLE habit_log_corrections;
DROP TABLE habit_logs;
ALTER TABLE habit_periods DROP CONSTRAINT habit_periods_owner_habit_id;
