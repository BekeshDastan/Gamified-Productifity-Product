-- +goose Up
INSERT INTO jobs(owner_id,kind,payload,dedupe_key,next_run_at,created_at)
SELECT owner_id,'seasons.close',jsonb_build_object('season_id',id::text),'season-close/'||owner_id::text||'/'||id::text,(ends_on+1)::timestamp AT TIME ZONE timezone,created_at FROM personal_seasons WHERE state IN('planned','active')
ON CONFLICT(dedupe_key) DO NOTHING;
-- +goose Down
-- Deadline rows are retained for queue audit; rollback requires stopped workers.
SELECT 1;
