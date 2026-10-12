-- +goose Up
CREATE TABLE onboarding (
    owner_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    step integer NOT NULL DEFAULT 0 CHECK (step BETWEEN 0 AND 6),
    completed_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version>0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
-- These are explicit drafts, not domain tasks/goals/habits or reward sources.
-- Future modules promote each draft once, preserving the stable source ID.
CREATE TABLE onboarding_starters (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL REFERENCES onboarding(owner_id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('goal','task','habit')),
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    promoted_entity_id uuid,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version>0),
    UNIQUE(owner_id,kind),
    UNIQUE(owner_id,id)
);
INSERT INTO onboarding(owner_id,created_at,updated_at) SELECT id,created_at,updated_at FROM users;
-- +goose StatementBegin
CREATE FUNCTION create_user_onboarding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO onboarding(owner_id,created_at,updated_at) VALUES(NEW.id,NEW.created_at,NEW.created_at);
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER users_create_onboarding AFTER INSERT ON users FOR EACH ROW EXECUTE FUNCTION create_user_onboarding();
-- +goose Down
DROP TRIGGER users_create_onboarding ON users;
DROP FUNCTION create_user_onboarding();
DROP TABLE onboarding_starters;
DROP TABLE onboarding;
