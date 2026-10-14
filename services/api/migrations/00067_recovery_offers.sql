-- +goose Up
CREATE TABLE recovery_offers (
 owner_id uuid NOT NULL REFERENCES users(id),id uuid NOT NULL DEFAULT gen_random_uuid(),
 snapshot jsonb NOT NULL,offered_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,
 state text NOT NULL DEFAULT 'offered' CHECK(state IN('offered','accepted','declined','withdrawn','expired')),
 version bigint NOT NULL DEFAULT 1,request jsonb,result jsonb,decided_at timestamptz,
 PRIMARY KEY(owner_id,id),
 CHECK((state='offered' AND decided_at IS NULL AND request IS NULL AND result IS NULL) OR (state IN('accepted','declined') AND decided_at IS NOT NULL AND request IS NOT NULL AND result IS NOT NULL) OR (state IN('withdrawn','expired') AND decided_at IS NOT NULL AND request IS NULL))
);
CREATE INDEX recovery_offer_history ON recovery_offers(owner_id,offered_at DESC,id DESC);
CREATE TABLE recovery_offer_events (
 owner_id uuid NOT NULL,offer_id uuid NOT NULL,version bigint NOT NULL,
 snapshot jsonb NOT NULL,created_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,offer_id,version),FOREIGN KEY(owner_id,offer_id) REFERENCES recovery_offers(owner_id,id)
);
CREATE TRIGGER recovery_events_immutable BEFORE UPDATE OR DELETE ON recovery_offer_events FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose StatementBegin
CREATE FUNCTION protect_recovery_offer() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'recovery history is retained'; END IF;
 IF ROW(OLD.owner_id,OLD.id,OLD.snapshot,OLD.offered_at,OLD.expires_at) IS DISTINCT FROM ROW(NEW.owner_id,NEW.id,NEW.snapshot,NEW.offered_at,NEW.expires_at) OR OLD.decided_at IS NOT NULL THEN RAISE EXCEPTION 'original recovery offer and decision are immutable'; END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER recovery_offer_guard BEFORE UPDATE OR DELETE ON recovery_offers FOR EACH ROW EXECUTE FUNCTION protect_recovery_offer();
-- +goose Down
DROP TABLE recovery_offer_events;
DROP TABLE recovery_offers;
DROP FUNCTION protect_recovery_offer();
