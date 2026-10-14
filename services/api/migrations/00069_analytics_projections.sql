-- +goose Up
CREATE TABLE analytics_projection_state (
 owner_id uuid PRIMARY KEY REFERENCES users(id), source_revision bigint NOT NULL DEFAULT 1,
 projected_revision bigint NOT NULL DEFAULT 0, timezone text NOT NULL DEFAULT 'UTC',
 requested_from date, requested_to date, computed_at timestamptz,
 CHECK ((requested_from IS NULL)=(requested_to IS NULL)),CHECK(requested_to IS NULL OR requested_to>requested_from)
);
CREATE TABLE daily_metrics (
 owner_id uuid NOT NULL REFERENCES users(id), local_date date NOT NULL, timezone text NOT NULL,
 source_revision bigint NOT NULL, projection_version text NOT NULL, computed_at timestamptz NOT NULL,
 payload jsonb NOT NULL, PRIMARY KEY(owner_id,local_date)
);
CREATE TABLE weekly_metrics (
 owner_id uuid NOT NULL REFERENCES users(id), week_start date NOT NULL, timezone text NOT NULL,
 source_revision bigint NOT NULL, projection_version text NOT NULL, computed_at timestamptz NOT NULL,
 payload jsonb NOT NULL, PRIMARY KEY(owner_id,week_start)
);
-- +goose Down
DROP TABLE weekly_metrics;
DROP TABLE daily_metrics;
DROP TABLE analytics_projection_state;
