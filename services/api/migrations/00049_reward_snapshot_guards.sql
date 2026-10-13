-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION protect_redemption_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR
 ROW(OLD.owner_id,OLD.id,OLD.kind,OLD.target_id,OLD.title,OLD.description,OLD.cost,OLD.definition_version,OLD.request,OLD.spend_ledger_id,OLD.created_at)
 IS DISTINCT FROM ROW(NEW.owner_id,NEW.id,NEW.kind,NEW.target_id,NEW.title,NEW.description,NEW.cost,NEW.definition_version,NEW.request,NEW.spend_ledger_id,NEW.created_at)
 OR OLD.status<>'pending' THEN RAISE EXCEPTION 'purchase snapshots and terminal decisions are permanent'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION protect_permanent_grant() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR
 ROW(OLD.owner_id,OLD.item_id,OLD.source_kind,OLD.source_key,OLD.granted_at)
 IS DISTINCT FROM ROW(NEW.owner_id,NEW.item_id,NEW.source_kind,NEW.source_key,NEW.granted_at)
 OR (OLD.source_kind<>'boss' AND OLD.active IS DISTINCT FROM NEW.active)
 THEN RAISE EXCEPTION 'permanent cosmetic grant cannot be rewritten'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER redemption_snapshot_immutable BEFORE UPDATE OR DELETE ON reward_redemptions FOR EACH ROW EXECUTE FUNCTION protect_redemption_snapshot();
CREATE TRIGGER permanent_grant_immutable BEFORE UPDATE OR DELETE ON cosmetic_permanent_grants FOR EACH ROW EXECUTE FUNCTION protect_permanent_grant();
-- +goose Down
DROP TRIGGER permanent_grant_immutable ON cosmetic_permanent_grants;
DROP TRIGGER redemption_snapshot_immutable ON reward_redemptions;
DROP FUNCTION protect_permanent_grant();
DROP FUNCTION protect_redemption_snapshot();
