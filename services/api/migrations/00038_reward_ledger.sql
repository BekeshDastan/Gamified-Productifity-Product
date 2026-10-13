-- +goose Up
CREATE TABLE user_progress (
 owner_id uuid PRIMARY KEY REFERENCES users(id),total_xp bigint NOT NULL DEFAULT 0 CHECK(total_xp BETWEEN 0 AND 9007199254740991),
 coins bigint NOT NULL DEFAULT 0,version bigint NOT NULL DEFAULT 1 CHECK(version>0),updated_at timestamptz NOT NULL
);
CREATE TABLE reward_entitlements (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner_id uuid NOT NULL REFERENCES users(id),source_kind text NOT NULL CHECK(source_kind IN('task','habit','challenge')),
 source_key text NOT NULL CHECK(length(source_key) BETWEEN 1 AND 200),economy_version text NOT NULL REFERENCES economy_versions(id),
 difficulty integer NOT NULL,area_id uuid,requested_xp bigint NOT NULL CHECK(requested_xp>=0),awarded_xp bigint NOT NULL CHECK(awarded_xp>=0 AND awarded_xp<=requested_xp),
 awarded_coins bigint NOT NULL CHECK(awarded_coins=awarded_xp/5),first_awarded_at timestamptz NOT NULL,active boolean NOT NULL,generation bigint NOT NULL CHECK(generation>0),
 UNIQUE(owner_id,id),UNIQUE(owner_id,source_kind,source_key),FOREIGN KEY(owner_id,area_id) REFERENCES life_areas(owner_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION reject_entitlement_snapshot_rewrite() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF (to_jsonb(OLD)-'active'-'generation') IS DISTINCT FROM (to_jsonb(NEW)-'active'-'generation') THEN RAISE EXCEPTION 'reward entitlement snapshot is immutable' USING ERRCODE='23514';END IF;RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER reward_entitlements_snapshot BEFORE UPDATE ON reward_entitlements FOR EACH ROW EXECUTE FUNCTION reject_entitlement_snapshot_rewrite();
CREATE TRIGGER reward_entitlements_no_delete BEFORE DELETE ON reward_entitlements FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
CREATE TABLE reward_quota_usage (
 owner_id uuid NOT NULL,entitlement_id uuid NOT NULL,source_kind text NOT NULL CHECK(source_kind IN('task','habit','challenge')),xp bigint NOT NULL CHECK(xp>=0),awarded_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,entitlement_id),FOREIGN KEY(owner_id,entitlement_id) REFERENCES reward_entitlements(owner_id,id)
);
CREATE INDEX reward_quota_window ON reward_quota_usage(owner_id,awarded_at,source_kind);
CREATE TRIGGER reward_quota_usage_immutable BEFORE UPDATE OR DELETE ON reward_quota_usage FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
CREATE TABLE reward_ledger (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner_id uuid NOT NULL REFERENCES users(id),entitlement_id uuid,
 operation_key text NOT NULL CHECK(length(operation_key) BETWEEN 1 AND 250),source_kind text NOT NULL CHECK(source_kind IN('task','habit','challenge','spend')),
 source_key text NOT NULL,kind text NOT NULL CHECK(kind IN('grant','reversal','restoration','spend')),economy_version text NOT NULL REFERENCES economy_versions(id),area_id uuid,
 xp_delta bigint NOT NULL,coins_delta bigint NOT NULL,reason text NOT NULL,occurred_at timestamptz NOT NULL,
 UNIQUE(owner_id,id),UNIQUE(owner_id,operation_key),FOREIGN KEY(owner_id,entitlement_id) REFERENCES reward_entitlements(owner_id,id),FOREIGN KEY(owner_id,area_id) REFERENCES life_areas(owner_id,id),
 CHECK((kind='spend' AND entitlement_id IS NULL AND source_kind='spend' AND xp_delta=0 AND coins_delta<=0) OR (kind IN('grant','restoration') AND entitlement_id IS NOT NULL AND xp_delta>=0 AND coins_delta>=0) OR (kind='reversal' AND entitlement_id IS NOT NULL AND xp_delta<=0 AND coins_delta<=0))
);
CREATE INDEX reward_ledger_history ON reward_ledger(owner_id,occurred_at DESC,id DESC);
CREATE TRIGGER reward_ledger_immutable BEFORE UPDATE OR DELETE ON reward_ledger FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE reward_ledger;
DROP TABLE reward_quota_usage;
DROP TABLE reward_entitlements;
DROP FUNCTION reject_entitlement_snapshot_rewrite();
DROP TABLE user_progress;
