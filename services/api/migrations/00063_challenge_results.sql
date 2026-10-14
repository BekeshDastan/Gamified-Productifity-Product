-- +goose Up
CREATE TABLE challenge_final_sources (
 owner_id uuid NOT NULL, participant_id uuid NOT NULL, source_kind text NOT NULL, source_key uuid NOT NULL,
 title text NOT NULL, source_url text NOT NULL, amount bigint NOT NULL,
 source_time timestamptz NOT NULL, source_start timestamptz NOT NULL, source_version bigint NOT NULL,
 active boolean NOT NULL, captured_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,participant_id,source_kind,source_key),
 FOREIGN KEY(owner_id,participant_id) REFERENCES challenge_participants(owner_id,id)
);
CREATE INDEX challenge_final_source_page ON challenge_final_sources(owner_id,participant_id,source_time DESC,source_key DESC);
CREATE TRIGGER challenge_final_sources_immutable BEFORE UPDATE OR DELETE ON challenge_final_sources FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- Existing open runs receive their one deadline job. No job row is rewritten
-- while holding an owner lock, avoiding the worker's job -> owner lock cycle.
INSERT INTO jobs(owner_id,kind,payload,dedupe_key,next_run_at,created_at)
SELECT owner_id,'challenges.close',jsonb_build_object('run_id',id::text),'challenge-close/'||owner_id::text||'/'||id::text,ends_at,created_at FROM challenge_runs WHERE state='open'
ON CONFLICT(dedupe_key) DO NOTHING;
-- +goose Down
DROP TABLE challenge_final_sources;
