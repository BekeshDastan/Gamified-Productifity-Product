-- +goose Up
CREATE TABLE auth_one_time_tokens (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind text NOT NULL CONSTRAINT auth_one_time_kind CHECK (kind IN ('verify_email')),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at > created_at),
    used_at timestamptz
);
CREATE INDEX auth_one_time_owner ON auth_one_time_tokens(user_id,kind);

-- +goose Down
DROP TABLE auth_one_time_tokens;
