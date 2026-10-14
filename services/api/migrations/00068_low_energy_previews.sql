-- +goose Up
CREATE TABLE low_energy_previews (
 owner_id uuid NOT NULL REFERENCES users(id), id uuid NOT NULL DEFAULT gen_random_uuid(),
 snapshot jsonb NOT NULL, created_at timestamptz NOT NULL, expires_at timestamptz NOT NULL,
 apply_request jsonb, result jsonb, applied_at timestamptz,
 PRIMARY KEY(owner_id,id), CHECK ((applied_at IS NULL AND apply_request IS NULL AND result IS NULL) OR (applied_at IS NOT NULL AND apply_request IS NOT NULL AND result IS NOT NULL))
);
-- +goose StatementBegin
CREATE FUNCTION protect_low_energy_preview() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.owner_id<>OLD.owner_id OR NEW.id<>OLD.id OR NEW.snapshot<>OLD.snapshot OR NEW.created_at<>OLD.created_at OR NEW.expires_at<>OLD.expires_at OR OLD.applied_at IS NOT NULL THEN
  RAISE EXCEPTION 'immutable low energy preview';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER low_energy_preview_immutable BEFORE UPDATE ON low_energy_previews FOR EACH ROW EXECUTE FUNCTION protect_low_energy_preview();
-- +goose Down
DROP TABLE low_energy_previews;
DROP FUNCTION protect_low_energy_preview();
