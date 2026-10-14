-- +goose Up
CREATE TABLE personal_seasons (
 owner_id uuid NOT NULL REFERENCES users(id), id uuid NOT NULL,
 request jsonb NOT NULL, title text NOT NULL, theme text NOT NULL, desired_result text NOT NULL,
 starts_on date NOT NULL, ends_on date NOT NULL, timezone text NOT NULL,
 is_primary boolean NOT NULL, state text NOT NULL DEFAULT 'planned' CHECK(state IN('planned','active','completed','cancelled')),
 version bigint NOT NULL DEFAULT 1, activated_at timestamptz, closed_at timestamptz,
 final_snapshot jsonb, archived_at timestamptz, created_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,id),CHECK(ends_on>=starts_on AND ends_on-starts_on<365)
);
CREATE UNIQUE INDEX one_primary_active_season ON personal_seasons(owner_id) WHERE state='active' AND is_primary;
CREATE INDEX season_page ON personal_seasons(owner_id,created_at DESC,id DESC);
CREATE TABLE personal_season_events (
 owner_id uuid NOT NULL, season_id uuid NOT NULL, version bigint NOT NULL,
 kind text NOT NULL, snapshot jsonb NOT NULL, created_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,season_id,version),FOREIGN KEY(owner_id,season_id) REFERENCES personal_seasons(owner_id,id)
);
CREATE TRIGGER season_events_immutable BEFORE UPDATE OR DELETE ON personal_season_events FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose StatementBegin
CREATE FUNCTION protect_personal_season() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'season history is retained'; END IF;
 IF ROW(OLD.owner_id,OLD.id,OLD.request,OLD.title,OLD.theme,OLD.desired_result,OLD.starts_on,OLD.ends_on,OLD.timezone,OLD.created_at) IS DISTINCT FROM ROW(NEW.owner_id,NEW.id,NEW.request,NEW.title,NEW.theme,NEW.desired_result,NEW.starts_on,NEW.ends_on,NEW.timezone,NEW.created_at) THEN RAISE EXCEPTION 'season definition is immutable'; END IF;
 IF OLD.final_snapshot IS NOT NULL AND OLD.final_snapshot IS DISTINCT FROM NEW.final_snapshot THEN RAISE EXCEPTION 'first season result is immutable'; END IF;
 IF OLD.state IN('completed','cancelled') AND OLD.state IS DISTINCT FROM NEW.state THEN RAISE EXCEPTION 'closed season cannot reopen'; END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER season_definition_guard BEFORE UPDATE OR DELETE ON personal_seasons FOR EACH ROW EXECUTE FUNCTION protect_personal_season();
-- +goose Down
DROP TABLE personal_season_events;
DROP TABLE personal_seasons;
DROP FUNCTION protect_personal_season();
