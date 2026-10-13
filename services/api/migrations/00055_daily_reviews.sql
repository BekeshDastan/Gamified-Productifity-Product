-- +goose Up
-- A review is the evening run for that date; the view prevents two competing
-- records and exposes only the owning scope for structured analytics.
CREATE VIEW daily_reviews AS SELECT owner_id,id,local_date,payload,version,created_at,updated_at FROM routine_runs WHERE kind='evening';
-- +goose Down
DROP VIEW daily_reviews;
