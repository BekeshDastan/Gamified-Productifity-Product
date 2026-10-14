-- +goose Up
CREATE TABLE smart_schedule_previews(owner_id uuid NOT NULL REFERENCES users(id),id uuid NOT NULL DEFAULT gen_random_uuid(),snapshot jsonb NOT NULL,expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL,PRIMARY KEY(owner_id,id));
CREATE TRIGGER smart_schedule_previews_immutable BEFORE UPDATE OR DELETE ON smart_schedule_previews FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
CREATE TABLE smart_schedule_applies(owner_id uuid NOT NULL REFERENCES users(id),operation_id uuid NOT NULL,preview_id uuid NOT NULL,request jsonb NOT NULL,result jsonb NOT NULL,created_at timestamptz NOT NULL,PRIMARY KEY(owner_id,operation_id),UNIQUE(owner_id,preview_id),FOREIGN KEY(owner_id,preview_id) REFERENCES smart_schedule_previews(owner_id,id));
CREATE TRIGGER smart_schedule_applies_immutable BEFORE UPDATE OR DELETE ON smart_schedule_applies FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE smart_schedule_applies;
DROP TABLE smart_schedule_previews;
