-- +goose Up
CREATE TABLE achievement_definitions(id text NOT NULL,version text NOT NULL,snapshot jsonb NOT NULL,PRIMARY KEY(id,version));
CREATE TRIGGER achievement_definitions_immutable BEFORE UPDATE OR DELETE ON achievement_definitions FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
CREATE TABLE user_achievements(owner_id uuid NOT NULL REFERENCES users(id),definition_id text NOT NULL,definition_version text NOT NULL,active boolean NOT NULL,awarded_at timestamptz NOT NULL,revoked_at timestamptz,evidence jsonb NOT NULL,PRIMARY KEY(owner_id,definition_id,definition_version),FOREIGN KEY(definition_id,definition_version) REFERENCES achievement_definitions(id,version));
CREATE TABLE achievement_changes(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,owner_id uuid NOT NULL,definition_id text NOT NULL,definition_version text NOT NULL,active boolean NOT NULL,evidence jsonb NOT NULL,created_at timestamptz NOT NULL,FOREIGN KEY(owner_id,definition_id,definition_version) REFERENCES user_achievements(owner_id,definition_id,definition_version));
CREATE TRIGGER achievement_changes_immutable BEFORE UPDATE OR DELETE ON achievement_changes FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE achievement_changes;
DROP TABLE user_achievements;
DROP TABLE achievement_definitions;
