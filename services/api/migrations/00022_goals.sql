-- +goose Up
CREATE TABLE goals (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL REFERENCES users(id),
 area_id uuid,
 payload jsonb NOT NULL,
 state text NOT NULL CHECK(state IN('draft','active','paused','achieved','not_achieved','cancelled')),
 start_date date,
 target_date date,
 progress numeric NOT NULL DEFAULT 0 CHECK(progress BETWEEN 0 AND 100),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 archived_at timestamptz,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 UNIQUE(owner_id,id),
 FOREIGN KEY(owner_id,area_id) REFERENCES life_areas(owner_id,id)
);
CREATE INDEX goals_owner_state ON goals(owner_id,state,created_at DESC,id);
CREATE TABLE goal_versions (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 owner_id uuid NOT NULL,
 goal_id uuid NOT NULL,
 goal_version bigint NOT NULL,
 kind text NOT NULL,
 reason text NOT NULL DEFAULT '',
 snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL,
 UNIQUE(goal_id,goal_version),
 FOREIGN KEY(owner_id,goal_id) REFERENCES goals(owner_id,id)
);
CREATE INDEX goal_versions_history ON goal_versions(owner_id,goal_id,id DESC);
-- +goose Down
DROP TABLE goal_versions;
DROP TABLE goals;
