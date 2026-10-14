-- +goose Up
CREATE TABLE accepted_quest_suggestions (
 owner_id uuid NOT NULL, period text NOT NULL CHECK(period IN('daily','weekly')),
 starts_on date NOT NULL, template_id text NOT NULL, catalog_version text NOT NULL,
 request jsonb NOT NULL, quest_id uuid NOT NULL, created_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,period,starts_on,template_id),
 FOREIGN KEY(owner_id,quest_id) REFERENCES quests(owner_id,id)
);
CREATE TRIGGER accepted_suggestion_immutable BEFORE UPDATE OR DELETE ON accepted_quest_suggestions FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE accepted_quest_suggestions;
