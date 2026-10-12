-- +goose Up
CREATE TABLE outbox_events (
    id uuid PRIMARY KEY,
    owner_id uuid,
    kind text NOT NULL CHECK (char_length(kind) BETWEEN 1 AND 100),
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    occurred_at timestamptz NOT NULL,
    dispatched_at timestamptz
);
CREATE INDEX outbox_pending ON outbox_events (occurred_at, id) WHERE dispatched_at IS NULL;
CREATE TABLE jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid,
    kind text NOT NULL CHECK (char_length(kind) BETWEEN 1 AND 100),
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    dedupe_key text NOT NULL UNIQUE CHECK (char_length(dedupe_key) BETWEEN 1 AND 200),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','running','succeeded','retry_wait','dead','cancelled')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 8),
    max_attempts integer NOT NULL DEFAULT 8 CHECK (max_attempts BETWEEN 1 AND 8),
    next_run_at timestamptz NOT NULL,
    lease_until timestamptz,
    lease_token uuid,
    last_error_code text,
    created_at timestamptz NOT NULL,
    completed_at timestamptz,
    CHECK ((state = 'running') = (lease_until IS NOT NULL AND lease_token IS NOT NULL))
);
CREATE INDEX jobs_due ON jobs (state, next_run_at, id);
CREATE INDEX jobs_owner ON jobs (owner_id);
-- +goose Down
DROP TABLE jobs;
DROP TABLE outbox_events;
