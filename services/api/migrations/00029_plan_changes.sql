-- +goose Up
CREATE TABLE plan_changes (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 owner_id uuid NOT NULL,
 plan_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN('confirmed','addendum','template')),
 before_state jsonb NOT NULL,
 after_state jsonb NOT NULL,
 reason text NOT NULL,
 created_at timestamptz NOT NULL,
 FOREIGN KEY(owner_id,plan_id) REFERENCES day_plans(owner_id,id)
);
CREATE INDEX plan_changes_plan ON plan_changes(owner_id,plan_id,id);
-- +goose Down
DROP TABLE plan_changes;
