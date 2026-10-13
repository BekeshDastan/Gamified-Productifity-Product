-- +goose Up
CREATE TABLE mystery_grants (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id),
 source text NOT NULL, source_version text NOT NULL, catalog_version text NOT NULL,
 weights jsonb NOT NULL, item_id text REFERENCES cosmetic_items(id), compensation bigint,
 created_at timestamptz NOT NULL, opened_at timestamptz,
 UNIQUE(owner_id,source,source_version),
 CHECK ((opened_at IS NULL AND item_id IS NULL AND compensation IS NULL) OR
        (opened_at IS NOT NULL AND ((item_id IS NOT NULL AND compensation IS NULL) OR (item_id IS NULL AND compensation=5))))
);
-- +goose StatementBegin
CREATE FUNCTION protect_mystery_result() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR OLD.opened_at IS NOT NULL OR
    ROW(OLD.id,OLD.owner_id,OLD.source,OLD.source_version,OLD.catalog_version,OLD.weights,OLD.created_at)
    IS DISTINCT FROM ROW(NEW.id,NEW.owner_id,NEW.source,NEW.source_version,NEW.catalog_version,NEW.weights,NEW.created_at)
 THEN RAISE EXCEPTION 'mystery grants and choices are permanent'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER mystery_result_immutable BEFORE UPDATE OR DELETE ON mystery_grants FOR EACH ROW EXECUTE FUNCTION protect_mystery_result();
CREATE INDEX mystery_grants_owner ON mystery_grants(owner_id,created_at,id);
-- +goose Down
DROP TABLE mystery_grants;
DROP FUNCTION protect_mystery_result();
