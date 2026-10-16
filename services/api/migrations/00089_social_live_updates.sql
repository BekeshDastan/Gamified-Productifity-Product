-- +goose Up
ALTER TABLE social_profiles ADD COLUMN revision uuid NOT NULL DEFAULT gen_random_uuid();
-- +goose Down
ALTER TABLE social_profiles DROP COLUMN revision;
