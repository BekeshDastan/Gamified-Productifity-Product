-- +goose Up
CREATE TABLE routine_runs (
 owner_id uuid NOT NULL REFERENCES users(id), id uuid NOT NULL DEFAULT gen_random_uuid(),
 local_date date NOT NULL, kind text NOT NULL CHECK(kind IN('morning','evening')),
 payload jsonb NOT NULL, version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,id), UNIQUE(owner_id,local_date,kind)
);
CREATE TABLE routine_run_events (
 owner_id uuid NOT NULL, run_id uuid NOT NULL, version bigint NOT NULL,
 snapshot jsonb NOT NULL, created_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,run_id,version), FOREIGN KEY(owner_id,run_id) REFERENCES routine_runs(owner_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION protect_routine_events() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'routine event snapshots are immutable'; END; $$;
-- +goose StatementEnd
CREATE TRIGGER routine_events_immutable BEFORE UPDATE OR DELETE ON routine_run_events FOR EACH ROW EXECUTE FUNCTION protect_routine_events();
-- +goose Down
DROP TABLE routine_run_events;
DROP TABLE routine_runs;
DROP FUNCTION protect_routine_events();
