-- +goose Up
CREATE TABLE recurrence_series_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 owner_id uuid NOT NULL,
 series_id uuid NOT NULL,
 before_state jsonb,
 after_state jsonb NOT NULL,
 reason text NOT NULL,
 created_at timestamptz NOT NULL,
 FOREIGN KEY(owner_id,series_id) REFERENCES recurrence_series(owner_id,id)
);
-- +goose Down
DROP TABLE recurrence_series_history;
