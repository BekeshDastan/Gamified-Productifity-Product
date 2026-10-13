-- +goose Up
CREATE TABLE custom_rewards(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner_id uuid NOT NULL REFERENCES users(id),title text NOT NULL,description text NOT NULL,cost bigint NOT NULL CHECK(cost BETWEEN 0 AND 1000000),available boolean NOT NULL,archived boolean NOT NULL DEFAULT false,version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,UNIQUE(owner_id,id));
CREATE TABLE custom_reward_versions(owner_id uuid NOT NULL,reward_id uuid NOT NULL,version bigint NOT NULL,snapshot jsonb NOT NULL,created_at timestamptz NOT NULL,PRIMARY KEY(owner_id,reward_id,version),FOREIGN KEY(owner_id,reward_id) REFERENCES custom_rewards(owner_id,id));
CREATE TRIGGER custom_reward_versions_immutable BEFORE UPDATE OR DELETE ON custom_reward_versions FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE custom_reward_versions;
DROP TABLE custom_rewards;
