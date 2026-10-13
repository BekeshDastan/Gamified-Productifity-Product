-- +goose Up
CREATE TABLE journal_entries (
 owner_id uuid NOT NULL REFERENCES users(id),
 id uuid NOT NULL DEFAULT gen_random_uuid(),
 local_date date NOT NULL,
 title text NOT NULL CHECK(char_length(title) BETWEEN 1 AND 200),
 body text NOT NULL CHECK(char_length(body)<=50000),
 tags text[] NOT NULL DEFAULT '{}' CHECK(cardinality(tags)<=20),
 goal_id uuid,
 archived boolean NOT NULL DEFAULT false,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,id),
 FOREIGN KEY(owner_id,goal_id) REFERENCES goals(owner_id,id)
);
CREATE INDEX journal_entries_date ON journal_entries(owner_id,local_date DESC,id);
-- +goose Down
DROP TABLE journal_entries;
