-- +goose Up
CREATE TABLE social_rooms (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id),
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 80),
 mode text NOT NULL CHECK(mode IN('coop','pvp')), metric text NOT NULL CHECK(metric IN('tasks','focus','reviews','manual')),
 unit text NOT NULL, target bigint NOT NULL CHECK(target>0), daily_cap bigint NOT NULL CHECK(daily_cap>0),
 starts_at timestamptz NOT NULL, ends_at timestamptz NOT NULL CHECK(ends_at>starts_at),
 rules_hash text NOT NULL, created_at timestamptz NOT NULL,
 finalized_at timestamptz, version bigint NOT NULL DEFAULT 1
);
CREATE TABLE social_members (
 room_id uuid NOT NULL REFERENCES social_rooms(id), owner_id uuid NOT NULL REFERENCES users(id),
 state text NOT NULL CHECK(state IN('invited','joined','left','rejected')),
 share boolean NOT NULL DEFAULT false, leaderboard boolean NOT NULL DEFAULT false,
 joined_at timestamptz, version bigint NOT NULL DEFAULT 1,
 PRIMARY KEY(room_id,owner_id)
);
CREATE TABLE social_checkins (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),room_id uuid NOT NULL,owner_id uuid NOT NULL,
 local_date date NOT NULL,amount bigint NOT NULL CHECK(amount>=0 AND amount<=1000),
 version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,
 FOREIGN KEY(room_id,owner_id) REFERENCES social_members(room_id,owner_id),
 UNIQUE(room_id,owner_id,local_date)
);
CREATE TABLE social_results (
 room_id uuid NOT NULL,owner_id uuid NOT NULL,amount bigint NOT NULL,rank integer NOT NULL,
 earned boolean NOT NULL,updated_at timestamptz NOT NULL,
 PRIMARY KEY(room_id,owner_id),FOREIGN KEY(room_id,owner_id) REFERENCES social_members(room_id,owner_id)
);
-- +goose StatementBegin
CREATE FUNCTION social_immutable_rules() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.owner_id,NEW.name,NEW.mode,NEW.metric,NEW.unit,NEW.target,NEW.daily_cap,NEW.starts_at,NEW.ends_at,NEW.rules_hash,NEW.created_at) IS DISTINCT FROM ROW(OLD.owner_id,OLD.name,OLD.mode,OLD.metric,OLD.unit,OLD.target,OLD.daily_cap,OLD.starts_at,OLD.ends_at,OLD.rules_hash,OLD.created_at) THEN
 RAISE EXCEPTION 'social rules are immutable';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER social_rules_fixed BEFORE UPDATE ON social_rooms FOR EACH ROW EXECUTE FUNCTION social_immutable_rules();
-- +goose Down
DROP TABLE social_results,social_checkins,social_members,social_rooms;
DROP FUNCTION social_immutable_rules();
