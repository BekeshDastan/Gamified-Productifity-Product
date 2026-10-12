-- +goose Up
CREATE TABLE auth_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_secret bytea NOT NULL CHECK (octet_length(csrf_secret) = 32),
    remember_me boolean NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at > created_at),
    revoked_at timestamptz,
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);
CREATE INDEX auth_sessions_user_id ON auth_sessions(user_id);

CREATE TABLE auth_refresh_tokens (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    session_id uuid NOT NULL REFERENCES auth_sessions(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    used_at timestamptz
);
CREATE INDEX auth_refresh_tokens_session_id ON auth_refresh_tokens(session_id);

-- +goose Down
DROP TABLE auth_refresh_tokens;
DROP TABLE auth_sessions;
