-- +goose Up
CREATE TABLE economy_versions(id text PRIMARY KEY,rules jsonb NOT NULL);
INSERT INTO economy_versions(id,rules) VALUES('economy-v1','{"task_xp":[5,10,25,50,100],"habit_xp":10,"challenge_xp":25,"quotas":{"task":200,"habit":50,"challenge":50,"total":300},"window_hours":24,"coins_divisor":5,"level_factor":100}');
-- +goose StatementBegin
CREATE FUNCTION reject_economy_rewrite() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'economy records are immutable' USING ERRCODE='23514'; END $$;
-- +goose StatementEnd
CREATE TRIGGER economy_versions_immutable BEFORE UPDATE OR DELETE ON economy_versions FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE economy_versions;
DROP FUNCTION reject_economy_rewrite();
