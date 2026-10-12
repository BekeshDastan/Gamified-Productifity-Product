-- +goose Up
CREATE TABLE audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id uuid,
    action text NOT NULL CHECK (char_length(action) BETWEEN 1 AND 100),
    entity_type text NOT NULL CHECK (char_length(entity_type) BETWEEN 1 AND 100),
    entity_id uuid,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_actor_created ON audit_events (actor_id, created_at, id);

-- +goose Down
DROP TABLE audit_events;
