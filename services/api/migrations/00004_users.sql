-- +goose Up
CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL CONSTRAINT users_email_key UNIQUE,
    password_hash text NOT NULL CHECK (length(password_hash) BETWEEN 80 AND 512),
    role text NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
    email_verified_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    disabled_at timestamptz,
    CONSTRAINT users_email_normalized CHECK (email = lower(btrim(email)) AND length(email) BETWEEN 3 AND 254)
);

-- +goose Down
DROP TABLE users;
