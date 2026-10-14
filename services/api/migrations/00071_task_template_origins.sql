-- +goose Up
CREATE TABLE task_template_origins (
 owner_id uuid NOT NULL,quest_id uuid NOT NULL,template_id uuid NOT NULL,
 template_name text NOT NULL,template_version bigint NOT NULL,template_input jsonb NOT NULL,created_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,quest_id),FOREIGN KEY(owner_id,quest_id) REFERENCES quests(owner_id,id)
);
CREATE INDEX task_template_samples ON task_template_origins(owner_id,template_id,template_version);
CREATE TRIGGER task_template_origins_immutable BEFORE UPDATE OR DELETE ON task_template_origins FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE task_template_origins;
