-- +goose Up
CREATE TABLE profiles (
    owner_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL DEFAULT '' CHECK (char_length(name) <= 80),
    nickname text NOT NULL DEFAULT '' CHECK (nickname = '' OR nickname ~ '^[a-z0-9_]{3,24}$'),
    avatar text NOT NULL DEFAULT 'mountain' CHECK (avatar IN ('mountain','compass','sprout','sun','moon','star')),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX profiles_nickname_unique ON profiles(nickname) WHERE nickname <> '';
INSERT INTO profiles(owner_id,created_at,updated_at) SELECT id,created_at,updated_at FROM users;
-- Profile creation shares the registration transaction, including future callers.
-- +goose StatementBegin
CREATE FUNCTION create_user_profile() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO profiles(owner_id,created_at,updated_at) VALUES(NEW.id,NEW.created_at,NEW.created_at);
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER users_create_profile AFTER INSERT ON users FOR EACH ROW EXECUTE FUNCTION create_user_profile();

-- +goose Down
DROP TRIGGER users_create_profile ON users;
DROP FUNCTION create_user_profile();
DROP TABLE profiles;
