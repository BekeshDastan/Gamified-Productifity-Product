-- +goose Up
CREATE TABLE weekly_review_previews (
 owner_id uuid NOT NULL REFERENCES users(id), id uuid NOT NULL DEFAULT gen_random_uuid(),
 local_date date NOT NULL, snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL, expires_at timestamptz NOT NULL,
 decision jsonb, result jsonb,
 PRIMARY KEY(owner_id,id), CHECK((decision IS NULL)=(result IS NULL))
);
CREATE TABLE weekly_reviews (
 owner_id uuid NOT NULL REFERENCES users(id), id uuid NOT NULL DEFAULT gen_random_uuid(),
 local_date date NOT NULL, version bigint NOT NULL DEFAULT 1, payload jsonb NOT NULL,
 updated_at timestamptz NOT NULL, PRIMARY KEY(owner_id,id), UNIQUE(owner_id,local_date)
);
-- +goose StatementBegin
CREATE FUNCTION protect_weekly_preview() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'weekly preview cannot be deleted'; END IF;
 IF (OLD.owner_id,OLD.id,OLD.local_date,OLD.snapshot,OLD.created_at,OLD.expires_at) IS DISTINCT FROM (NEW.owner_id,NEW.id,NEW.local_date,NEW.snapshot,NEW.created_at,NEW.expires_at) OR OLD.result IS NOT NULL THEN RAISE EXCEPTION 'weekly decision is immutable'; END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER weekly_preview_guard BEFORE UPDATE OR DELETE ON weekly_review_previews FOR EACH ROW EXECUTE FUNCTION protect_weekly_preview();
-- +goose Down
DROP TABLE weekly_reviews;
DROP TABLE weekly_review_previews;
DROP FUNCTION protect_weekly_preview();
