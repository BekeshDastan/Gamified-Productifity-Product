-- +goose Up
CREATE TABLE user_settings (
    owner_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    preferences jsonb NOT NULL DEFAULT '{"timezone":"UTC","language":"ru","week_start":1,"time_format":"24h","work_intervals":[],"rest_days":[6,7]}'::jsonb CHECK (jsonb_typeof(preferences)='object'),
    version bigint NOT NULL DEFAULT 1 CHECK (version>0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
INSERT INTO user_settings(owner_id,created_at,updated_at) SELECT id,created_at,updated_at FROM users;
-- +goose StatementBegin
CREATE FUNCTION create_user_settings() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO user_settings(owner_id,created_at,updated_at) VALUES(NEW.id,NEW.created_at,NEW.created_at);
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER users_create_settings AFTER INSERT ON users FOR EACH ROW EXECUTE FUNCTION create_user_settings();
-- +goose Down
DROP TRIGGER users_create_settings ON users;
DROP FUNCTION create_user_settings();
DROP TABLE user_settings;
