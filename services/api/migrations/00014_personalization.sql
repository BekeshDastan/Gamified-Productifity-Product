-- +goose Up
ALTER TABLE user_settings ALTER COLUMN preferences SET DEFAULT '{"timezone":"UTC","language":"ru","week_start":1,"time_format":"24h","work_intervals":[],"rest_days":[6,7],"privacy":{"profile_visibility":"private","share_activity":false,"share_achievements":false,"discoverable":false},"personalization":{"theme":"system","motion":"system","sound":false,"game_mode":"calm","widget_order":["today","calendar","habits","focus","progress"]}}'::jsonb;
UPDATE user_settings SET preferences=preferences || '{"personalization":{"theme":"system","motion":"system","sound":false,"game_mode":"calm","widget_order":["today","calendar","habits","focus","progress"]}}'::jsonb,version=version+1;
-- +goose Down
UPDATE user_settings SET preferences=preferences-'personalization',version=version+1;
ALTER TABLE user_settings ALTER COLUMN preferences SET DEFAULT '{"timezone":"UTC","language":"ru","week_start":1,"time_format":"24h","work_intervals":[],"rest_days":[6,7],"privacy":{"profile_visibility":"private","share_activity":false,"share_achievements":false,"discoverable":false}}'::jsonb;
