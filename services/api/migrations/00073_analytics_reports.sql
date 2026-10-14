-- +goose Up
CREATE TABLE report_previews (
 owner_id uuid NOT NULL REFERENCES users(id),id uuid NOT NULL DEFAULT gen_random_uuid(),snapshot jsonb NOT NULL,
 expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL,PRIMARY KEY(owner_id,id)
);
CREATE TABLE report_snapshots (
 owner_id uuid NOT NULL REFERENCES users(id),id uuid NOT NULL,preview_id uuid,request jsonb,
 kind text NOT NULL CHECK(kind IN('week','month')),starts_on date NOT NULL,ends_before date NOT NULL,
 timezone text NOT NULL,revision bigint NOT NULL,automatic boolean NOT NULL,snapshot jsonb NOT NULL,created_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,id),UNIQUE(owner_id,preview_id),UNIQUE(owner_id,kind,starts_on,timezone,revision),
 FOREIGN KEY(owner_id,preview_id) REFERENCES report_previews(owner_id,id),
 CHECK ((automatic AND request IS NULL AND preview_id IS NULL) OR (NOT automatic AND request IS NOT NULL AND preview_id IS NOT NULL))
);
CREATE UNIQUE INDEX automatic_report_once ON report_snapshots(owner_id,kind,starts_on,timezone) WHERE automatic;
CREATE INDEX report_history ON report_snapshots(owner_id,created_at DESC,id DESC);
CREATE TRIGGER report_previews_immutable BEFORE UPDATE OR DELETE ON report_previews FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
CREATE TRIGGER report_snapshots_immutable BEFORE UPDATE OR DELETE ON report_snapshots FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE report_snapshots;
DROP TABLE report_previews;
