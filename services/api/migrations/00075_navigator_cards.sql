-- +goose Up
CREATE TABLE navigator_rule_preferences(owner_id uuid NOT NULL REFERENCES users(id),rule_id text NOT NULL,disabled boolean NOT NULL,version bigint NOT NULL DEFAULT 1,PRIMARY KEY(owner_id,rule_id));
CREATE TABLE navigator_cards(owner_id uuid NOT NULL REFERENCES users(id),id uuid NOT NULL DEFAULT gen_random_uuid(),rule_id text NOT NULL,rule_version bigint NOT NULL,subject text NOT NULL,created_at timestamptz NOT NULL,hidden boolean NOT NULL DEFAULT false,snoozed_until timestamptz,version bigint NOT NULL DEFAULT 1,PRIMARY KEY(owner_id,id),FOREIGN KEY(rule_id,rule_version) REFERENCES navigator_rule_versions(id,version));
CREATE INDEX navigator_cards_cooldown ON navigator_cards(owner_id,rule_id,subject,created_at DESC,id DESC);
CREATE TABLE navigator_task_views(owner_id uuid NOT NULL,quest_id uuid NOT NULL,last_seen timestamptz NOT NULL,visible boolean NOT NULL,visible_seconds integer NOT NULL DEFAULT 0 CHECK(visible_seconds>=0),PRIMARY KEY(owner_id,quest_id),FOREIGN KEY(owner_id,quest_id) REFERENCES quests(owner_id,id));
-- +goose Down
DROP TABLE navigator_task_views;
DROP TABLE navigator_cards;
DROP TABLE navigator_rule_preferences;
