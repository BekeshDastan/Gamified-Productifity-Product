-- +goose Up
CREATE TABLE day_plans (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL REFERENCES users(id),
 local_date date NOT NULL,
 timezone text NOT NULL,
 payload jsonb NOT NULL,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 confirmed_at timestamptz,
 snapshot jsonb,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 UNIQUE(owner_id,local_date), UNIQUE(owner_id,id),
 CHECK((confirmed_at IS NULL)=(snapshot IS NULL))
);
CREATE TABLE day_plan_items (
 owner_id uuid NOT NULL,
 plan_id uuid NOT NULL,
 quest_id uuid NOT NULL,
 position integer NOT NULL CHECK(position>=0),
 PRIMARY KEY(owner_id,plan_id,quest_id),
 FOREIGN KEY(owner_id,plan_id) REFERENCES day_plans(owner_id,id),
 FOREIGN KEY(owner_id,quest_id) REFERENCES quests(owner_id,id)
);
-- +goose Down
DROP TABLE day_plan_items;
DROP TABLE day_plans;
