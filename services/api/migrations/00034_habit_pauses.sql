-- +goose Up
CREATE TABLE habit_pauses (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner_id uuid NOT NULL,habit_id uuid NOT NULL,starts_on date NOT NULL,ends_on date,reason text NOT NULL,
 created_at timestamptz NOT NULL,version bigint NOT NULL DEFAULT 1,
 CHECK(ends_on IS NULL OR ends_on>=starts_on),UNIQUE(owner_id,id),FOREIGN KEY(owner_id,habit_id) REFERENCES habits(owner_id,id)
);
ALTER TABLE habit_periods ADD COLUMN rest_reason text NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE habit_periods DROP COLUMN rest_reason;
DROP TABLE habit_pauses;
