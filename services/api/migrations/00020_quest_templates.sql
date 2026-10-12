-- +goose Up
CREATE TABLE quest_templates (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 80),
 payload jsonb NOT NULL,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX quest_templates_owner_name ON quest_templates(owner_id,lower(name));
-- +goose Down
DROP TABLE quest_templates;
