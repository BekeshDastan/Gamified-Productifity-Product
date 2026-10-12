-- +goose Up
CREATE TABLE milestones (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL,
 goal_id uuid NOT NULL,
 title text NOT NULL CHECK(char_length(title) BETWEEN 1 AND 200),
 required boolean NOT NULL,
 weight numeric NOT NULL CHECK(weight>0 AND weight<=1000),
 completed boolean NOT NULL DEFAULT false,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL,
 UNIQUE(owner_id,goal_id,id),
 FOREIGN KEY(owner_id,goal_id) REFERENCES goals(owner_id,id)
);
CREATE TABLE goal_quests (
 owner_id uuid NOT NULL,
 goal_id uuid NOT NULL,
 quest_id uuid PRIMARY KEY,
 milestone_id uuid,
 required boolean NOT NULL,
 weight numeric NOT NULL CHECK(weight>0 AND weight<=1000),
 FOREIGN KEY(owner_id,goal_id) REFERENCES goals(owner_id,id),
 FOREIGN KEY(owner_id,quest_id) REFERENCES quests(owner_id,id),
 FOREIGN KEY(owner_id,goal_id,milestone_id) REFERENCES milestones(owner_id,goal_id,id)
);
CREATE INDEX goal_quests_goal ON goal_quests(owner_id,goal_id);
-- +goose Down
DROP TABLE goal_quests;
DROP TABLE milestones;
