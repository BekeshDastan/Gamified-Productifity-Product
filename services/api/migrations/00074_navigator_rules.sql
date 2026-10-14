-- +goose Up
CREATE TABLE navigator_rule_versions(id text NOT NULL CHECK(id ~ '^R(0[1-9]|1[0-5])$'),version bigint NOT NULL CHECK(version>0),payload jsonb NOT NULL,PRIMARY KEY(id,version));
CREATE TRIGGER navigator_rules_immutable BEFORE UPDATE OR DELETE ON navigator_rule_versions FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
INSERT INTO navigator_rule_versions(id,version,payload) VALUES
('R01',1,'{"id":"R01","version":1,"priority":75,"conditions":[{"metric":"postponements","op":"gte","value":3},{"metric":"window_days","op":"eq","value":14}],"template_key":"R01","action_type":"task_steps","cooldown_seconds":604800,"enabled":true}'::jsonb),
('R02',1,'{"id":"R02","version":1,"priority":90,"conditions":[{"metric":"deficit_minutes","op":"gt","value":0}],"template_key":"R02","action_type":"plan_review","cooldown_seconds":86400,"enabled":true}'::jsonb),
('R03',1,'{"id":"R03","version":1,"priority":100,"conditions":[{"metric":"hours_to_deadline","op":"gt","value":0},{"metric":"hours_to_deadline","op":"lt","value":24},{"metric":"deficit_minutes","op":"gt","value":0}],"template_key":"R03","action_type":"deadline_radar","cooldown_seconds":86400,"enabled":true}'::jsonb),
('R04',1,'{"id":"R04","version":1,"priority":65,"conditions":[{"metric":"missed_periods","op":"gte","value":2}],"template_key":"R04","action_type":"habit_review","cooldown_seconds":604800,"enabled":true}'::jsonb),
('R05',1,'{"id":"R05","version":1,"priority":70,"conditions":[{"metric":"inactive_days","op":"gte","value":3},{"metric":"returned","op":"eq","value":1}],"template_key":"R05","action_type":"comeback","cooldown_seconds":1209600,"enabled":true}'::jsonb),
('R06',1,'{"id":"R06","version":1,"priority":85,"conditions":[{"metric":"low_energy","op":"eq","value":1}],"template_key":"R06","action_type":"low_energy","cooldown_seconds":86400,"enabled":true}'::jsonb),
('R07',1,'{"id":"R07","version":1,"priority":60,"conditions":[{"metric":"visible_seconds","op":"gt","value":3600},{"metric":"first_steps","op":"eq","value":0}],"template_key":"R07","action_type":"five_minute_start","cooldown_seconds":86400,"enabled":true}'::jsonb),
('R08',1,'{"id":"R08","version":1,"priority":55,"conditions":[{"metric":"review_missing","op":"eq","value":1}],"template_key":"R08","action_type":"weekly_reset","cooldown_seconds":604800,"enabled":true}'::jsonb),
('R09',1,'{"id":"R09","version":1,"priority":50,"conditions":[{"metric":"samples","op":"gte","value":5},{"metric":"actual_estimate_ratio","op":"gt","value":1.5}],"template_key":"R09","action_type":"template_estimate","cooldown_seconds":604800,"enabled":true}'::jsonb),
('R10',1,'{"id":"R10","version":1,"priority":45,"conditions":[{"metric":"next_actions","op":"eq","value":0}],"template_key":"R10","action_type":"goal_first_action","cooldown_seconds":604800,"enabled":true}'::jsonb),
('R11',1,'{"id":"R11","version":1,"priority":40,"conditions":[{"metric":"streak","op":"gte","value":7}],"template_key":"R11","action_type":"achievements","cooldown_seconds":604800,"enabled":true}'::jsonb),
('R12',1,'{"id":"R12","version":1,"priority":95,"conditions":[{"metric":"overlap_minutes","op":"gt","value":0}],"template_key":"R12","action_type":"calendar_conflict","cooldown_seconds":86400,"enabled":true}'::jsonb),
('R13',1,'{"id":"R13","version":1,"priority":80,"conditions":[{"metric":"overdue","op":"eq","value":1},{"metric":"progress","op":"gt","value":0}],"template_key":"R13","action_type":"goal_review","cooldown_seconds":604800,"enabled":true}'::jsonb),
('R14',1,'{"id":"R14","version":1,"priority":30,"conditions":[{"metric":"boss_missing","op":"eq","value":1}],"template_key":"R14","action_type":"monthly_boss","cooldown_seconds":604800,"enabled":true}'::jsonb),
('R15',1,'{"id":"R15","version":1,"priority":88,"conditions":[{"metric":"pending_sessions","op":"gt","value":0}],"template_key":"R15","action_type":"focus_confirmation","cooldown_seconds":86400,"enabled":true}'::jsonb);
-- +goose Down
DROP TABLE navigator_rule_versions;
