-- +goose Up
CREATE TABLE quests (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 area_id uuid,
 payload jsonb NOT NULL,
 status text NOT NULL CHECK(status IN ('backlog','todo','in_progress','postponed','completed','failed','skipped')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 completed_at timestamptz,
 archived_at timestamptz,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 UNIQUE(owner_id,id),
 FOREIGN KEY(owner_id,area_id) REFERENCES life_areas(owner_id,id)
);
CREATE INDEX quests_owner_order ON quests(owner_id,created_at DESC,id);
CREATE INDEX quests_owner_status ON quests(owner_id,status) WHERE archived_at IS NULL;
CREATE TABLE quest_tags (
 owner_id uuid NOT NULL,
 quest_id uuid NOT NULL,
 tag_id uuid NOT NULL,
 PRIMARY KEY(quest_id,tag_id),
 FOREIGN KEY(owner_id,quest_id) REFERENCES quests(owner_id,id) ON DELETE CASCADE,
 FOREIGN KEY(owner_id,tag_id) REFERENCES tags(owner_id,id)
);
-- +goose Down
DROP TABLE quest_tags;
DROP TABLE quests;
