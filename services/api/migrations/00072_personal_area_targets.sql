-- +goose Up
CREATE TABLE personal_area_targets (
 owner_id uuid NOT NULL,area_id uuid NOT NULL,starts_on date NOT NULL,ends_before date NOT NULL,
 metric text NOT NULL CHECK(metric IN('tasks','focus_minutes','habit_norms')),target numeric(15,3) NOT NULL CHECK(target BETWEEN 0 AND 1000000),
 version bigint NOT NULL DEFAULT 1,updated_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,area_id,starts_on,ends_before),FOREIGN KEY(owner_id,area_id) REFERENCES life_areas(owner_id,id),
 CHECK(ends_before-starts_on BETWEEN 1 AND 366)
);
CREATE TABLE personal_area_target_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,owner_id uuid NOT NULL,area_id uuid NOT NULL,
 starts_on date NOT NULL,ends_before date NOT NULL,version bigint NOT NULL,snapshot jsonb NOT NULL,created_at timestamptz NOT NULL,
 UNIQUE(owner_id,area_id,starts_on,ends_before,version),FOREIGN KEY(owner_id,area_id) REFERENCES life_areas(owner_id,id)
);
CREATE TRIGGER personal_area_target_events_immutable BEFORE UPDATE OR DELETE ON personal_area_target_events FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE personal_area_target_events;
DROP TABLE personal_area_targets;
