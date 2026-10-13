-- +goose Up
ALTER TABLE habit_periods ADD COLUMN emergency_at timestamptz;
-- An emergency reason documents an existing obligation; only planned future rest excludes it.
UPDATE habit_periods SET excluded=false WHERE emergency_reason<>'' AND rest_reason='';
-- +goose Down
ALTER TABLE habit_periods DROP COLUMN emergency_at;
