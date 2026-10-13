-- +goose Up
CREATE TABLE reward_projection_events (
 owner_id uuid NOT NULL,ledger_id uuid NOT NULL,processed_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,ledger_id),FOREIGN KEY(owner_id,ledger_id) REFERENCES reward_ledger(owner_id,id)
);
-- +goose Down
DROP TABLE reward_projection_events;
