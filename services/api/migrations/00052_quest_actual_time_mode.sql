-- +goose Up
UPDATE quests SET payload=jsonb_set(payload,'{actual_time_mode}',to_jsonb(CASE WHEN COALESCE((payload->>'actual_minutes')::integer,0)>0 THEN 'manual_override'::text ELSE 'from_focus'::text END)) WHERE NOT(payload ? 'actual_time_mode');
ALTER TABLE quests ADD CONSTRAINT quest_actual_time_mode CHECK(payload->>'actual_time_mode' IN('from_focus','manual_override'));
-- +goose Down
ALTER TABLE quests DROP CONSTRAINT quest_actual_time_mode;
UPDATE quests SET payload=payload-'actual_time_mode';
