-- +goose Up
CREATE TABLE bosses(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner_id uuid NOT NULL REFERENCES users(id),goal_id uuid NOT NULL,state text NOT NULL CHECK(state IN('active','paused','closed')),version bigint NOT NULL DEFAULT 1,last_goal_version bigint NOT NULL DEFAULT 0,victory_goal_version bigint,won_at timestamptz,created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,UNIQUE(owner_id,id),UNIQUE(owner_id,goal_id),FOREIGN KEY(owner_id,goal_id) REFERENCES goals(owner_id,id));
CREATE TABLE boss_events(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,owner_id uuid NOT NULL,boss_id uuid NOT NULL,snapshot jsonb NOT NULL,reason text NOT NULL,created_at timestamptz NOT NULL,FOREIGN KEY(owner_id,boss_id) REFERENCES bosses(owner_id,id));
CREATE TRIGGER boss_events_immutable BEFORE UPDATE OR DELETE ON boss_events FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE boss_events;
DROP TABLE bosses;
