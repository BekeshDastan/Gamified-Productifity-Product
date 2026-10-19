-- +goose Up
CREATE TABLE runtime_heartbeats (
 name text PRIMARY KEY CHECK (name = 'worker'),
 seen_at timestamptz NOT NULL
);
-- +goose Down
DROP TABLE runtime_heartbeats;
