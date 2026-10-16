-- +goose Up
CREATE TABLE social_nudges (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),sender uuid NOT NULL REFERENCES users(id),recipient uuid NOT NULL REFERENCES users(id),
 template text NOT NULL CHECK(template IN('small_step','rest','celebrate')),created_at timestamptz NOT NULL
);
CREATE INDEX social_nudge_limits ON social_nudges(recipient,created_at);
CREATE TABLE social_posts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner_id uuid NOT NULL REFERENCES users(id),
 source_key text NOT NULL,source_version text NOT NULL,hidden boolean NOT NULL DEFAULT false,
 moderated boolean NOT NULL DEFAULT false,created_at timestamptz NOT NULL,UNIQUE(owner_id,source_key,source_version)
);
-- +goose Down
DROP TABLE social_posts,social_nudges;
