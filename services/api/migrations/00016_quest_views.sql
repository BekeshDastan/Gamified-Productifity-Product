-- +goose Up
CREATE TABLE quest_views (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 60),
 filters jsonb NOT NULL,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL,
 UNIQUE(owner_id,name)
);
-- +goose Down
DROP TABLE quest_views;
