-- +goose Up
ALTER TABLE reward_ledger ADD COLUMN sequence bigint GENERATED ALWAYS AS IDENTITY;
ALTER TABLE reward_ledger ADD CONSTRAINT reward_ledger_sequence_unique UNIQUE(sequence);
CREATE INDEX reward_ledger_owner_sequence ON reward_ledger(owner_id,sequence DESC);
-- +goose Down
DROP INDEX reward_ledger_owner_sequence;
ALTER TABLE reward_ledger DROP COLUMN sequence;
