-- +goose Up
CREATE TABLE monthly_bosses(owner_id uuid NOT NULL REFERENCES users(id),month text NOT NULL CHECK(month ~ '^\d{4}-\d{2}$'),timezone text NOT NULL,boss_id uuid,title text NOT NULL DEFAULT '',version bigint NOT NULL DEFAULT 1,updated_at timestamptz NOT NULL,PRIMARY KEY(owner_id,month),FOREIGN KEY(owner_id,boss_id) REFERENCES bosses(owner_id,id));
CREATE TABLE monthly_boss_changes(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,owner_id uuid NOT NULL,month text NOT NULL,timezone text NOT NULL,boss_id uuid,title text NOT NULL,reason text NOT NULL,created_at timestamptz NOT NULL,FOREIGN KEY(owner_id,month) REFERENCES monthly_bosses(owner_id,month),FOREIGN KEY(owner_id,boss_id) REFERENCES bosses(owner_id,id));
CREATE INDEX monthly_boss_changes_owner ON monthly_boss_changes(owner_id,id DESC);
CREATE TRIGGER monthly_boss_changes_immutable BEFORE UPDATE OR DELETE ON monthly_boss_changes FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE monthly_boss_changes;
DROP TABLE monthly_bosses;
