-- +goose Up
ALTER TABLE quests ADD COLUMN parent_id uuid;
ALTER TABLE quests ADD CONSTRAINT quests_parent_owner FOREIGN KEY(owner_id,parent_id) REFERENCES quests(owner_id,id);
ALTER TABLE quests ADD CONSTRAINT quests_not_self_parent CHECK(parent_id IS DISTINCT FROM id);
UPDATE quests SET payload=payload||'{"parent_id":null,"required":true}';
UPDATE task_events SET after_state=after_state||'{"parent_id":null,"required":true}',
 before_state=CASE WHEN before_state IS NULL OR before_state='null'::jsonb THEN before_state ELSE before_state||'{"parent_id":null,"required":true}' END;
CREATE INDEX quests_parent ON quests(owner_id,parent_id);
CREATE TABLE quest_dependencies (
 owner_id uuid NOT NULL,
 quest_id uuid NOT NULL,
 depends_on uuid NOT NULL,
 PRIMARY KEY(quest_id,depends_on),
 CHECK(quest_id<>depends_on),
 FOREIGN KEY(owner_id,quest_id) REFERENCES quests(owner_id,id),
 FOREIGN KEY(owner_id,depends_on) REFERENCES quests(owner_id,id)
);
-- +goose Down
DROP TABLE quest_dependencies;
ALTER TABLE quests DROP CONSTRAINT quests_not_self_parent;
ALTER TABLE quests DROP CONSTRAINT quests_parent_owner;
ALTER TABLE quests DROP COLUMN parent_id;
