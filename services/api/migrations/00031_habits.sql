-- +goose Up
CREATE TABLE habits (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL REFERENCES users(id),
 payload jsonb NOT NULL,
 area_id uuid,
 FOREIGN KEY(owner_id,area_id) REFERENCES life_areas(owner_id,id),
 timezone text NOT NULL,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 archived_at timestamptz,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 UNIQUE(owner_id,id)
);
CREATE TABLE habit_schedules (
 owner_id uuid NOT NULL,
 habit_id uuid NOT NULL,
 version bigint NOT NULL,
 effective_from date NOT NULL,
 schedule jsonb NOT NULL,
 reason text NOT NULL,
 PRIMARY KEY(owner_id,habit_id,version),
 UNIQUE(owner_id,habit_id,effective_from),
 FOREIGN KEY(owner_id,habit_id) REFERENCES habits(owner_id,id)
);
-- +goose Down
DROP TABLE habit_schedules;
DROP TABLE habits;
