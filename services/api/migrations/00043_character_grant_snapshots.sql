-- +goose Up
CREATE TRIGGER cosmetic_items_immutable BEFORE UPDATE OR DELETE ON cosmetic_items FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
CREATE TRIGGER character_inventory_events_immutable BEFORE UPDATE OR DELETE ON character_inventory_events FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose StatementBegin
CREATE FUNCTION reject_character_grant_rewrite() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_TABLE_NAME='character_inventory' THEN
  IF (to_jsonb(OLD)-ARRAY['active','revoked_at']) IS DISTINCT FROM (to_jsonb(NEW)-ARRAY['active','revoked_at']) THEN RAISE EXCEPTION 'inventory grant snapshot is immutable' USING ERRCODE='23514';END IF;
 ELSE
  IF (to_jsonb(OLD)-ARRAY['active','revoked_at','evidence']) IS DISTINCT FROM (to_jsonb(NEW)-ARRAY['active','revoked_at','evidence']) THEN RAISE EXCEPTION 'achievement grant snapshot is immutable' USING ERRCODE='23514';END IF;
 END IF;RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER character_inventory_snapshot BEFORE UPDATE ON character_inventory FOR EACH ROW EXECUTE FUNCTION reject_character_grant_rewrite();
CREATE TRIGGER user_achievements_snapshot BEFORE UPDATE ON user_achievements FOR EACH ROW EXECUTE FUNCTION reject_character_grant_rewrite();
CREATE TRIGGER character_inventory_no_delete BEFORE DELETE ON character_inventory FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
CREATE TRIGGER user_achievements_no_delete BEFORE DELETE ON user_achievements FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TRIGGER user_achievements_no_delete ON user_achievements;
DROP TRIGGER character_inventory_no_delete ON character_inventory;
DROP TRIGGER user_achievements_snapshot ON user_achievements;
DROP TRIGGER character_inventory_snapshot ON character_inventory;
DROP FUNCTION reject_character_grant_rewrite();
DROP TRIGGER character_inventory_events_immutable ON character_inventory_events;
DROP TRIGGER cosmetic_items_immutable ON cosmetic_items;
