-- +goose Up
CREATE TABLE attachments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id uuid NOT NULL,
 quest_id uuid NOT NULL,
 storage_key text NOT NULL UNIQUE CHECK(storage_key ~ '^[a-f0-9]{64}$'),
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 180),
 mime text NOT NULL,
 size bigint NOT NULL CHECK(size BETWEEN 1 AND 10485760),
 checksum text NOT NULL,
 verdict text NOT NULL DEFAULT 'pending' CHECK(verdict IN ('pending','clean','rejected')),
 deleted_at timestamptz,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL,
 FOREIGN KEY(owner_id,quest_id) REFERENCES quests(owner_id,id)
);
CREATE INDEX attachments_owner ON attachments(owner_id,quest_id) WHERE deleted_at IS NULL;
-- +goose Down
DROP TABLE attachments;
