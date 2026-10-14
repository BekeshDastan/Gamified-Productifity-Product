-- +goose Up
CREATE TABLE challenge_runs (
 owner_id uuid NOT NULL REFERENCES users(id), id uuid NOT NULL,
 request jsonb NOT NULL, rules jsonb NOT NULL, rules_hash text NOT NULL,
 catalog_id text, catalog_version text NOT NULL,
 state text NOT NULL DEFAULT 'open' CHECK(state IN('open','closed')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 starts_at timestamptz NOT NULL, ends_at timestamptz NOT NULL CHECK(ends_at>starts_at),
 created_at timestamptz NOT NULL, finalized_at timestamptz,
 final_snapshot jsonb, PRIMARY KEY(owner_id,id)
);
CREATE INDEX challenge_runs_owner_time ON challenge_runs(owner_id,created_at DESC,id DESC);
CREATE INDEX challenge_runs_ending ON challenge_runs(ends_at) WHERE state='open';
-- +goose StatementBegin
CREATE FUNCTION protect_challenge_rules() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'challenge run history is retained'; END IF;
 IF (OLD.owner_id,OLD.id,OLD.request,OLD.rules,OLD.rules_hash,OLD.catalog_id,OLD.catalog_version,OLD.starts_at,OLD.ends_at,OLD.created_at) IS DISTINCT FROM (NEW.owner_id,NEW.id,NEW.request,NEW.rules,NEW.rules_hash,NEW.catalog_id,NEW.catalog_version,NEW.starts_at,NEW.ends_at,NEW.created_at) THEN RAISE EXCEPTION 'challenge scoring rules are immutable'; END IF;
 IF OLD.final_snapshot IS NOT NULL AND OLD.final_snapshot IS DISTINCT FROM NEW.final_snapshot THEN RAISE EXCEPTION 'initial challenge result is immutable'; END IF;
 IF OLD.state='closed' AND NEW.state<>'closed' THEN RAISE EXCEPTION 'closed challenge cannot reopen'; END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER challenge_rules_immutable BEFORE UPDATE OR DELETE ON challenge_runs FOR EACH ROW EXECUTE FUNCTION protect_challenge_rules();
-- +goose Down
DROP TABLE challenge_runs;
DROP FUNCTION protect_challenge_rules();
