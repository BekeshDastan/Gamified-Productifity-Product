-- +goose Up
CREATE TABLE challenge_source_credits (
 owner_id uuid NOT NULL, participant_id uuid NOT NULL,
 source_kind text NOT NULL CHECK(source_kind IN('task','habit','focus','review','stage','manual')),
 source_key uuid NOT NULL, title text NOT NULL, source_url text NOT NULL,
 amount bigint NOT NULL CHECK(amount>0), source_time timestamptz NOT NULL,
 source_start timestamptz NOT NULL, source_version bigint NOT NULL,
 active boolean NOT NULL DEFAULT true, updated_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,participant_id,source_kind,source_key),
 FOREIGN KEY(owner_id,participant_id) REFERENCES challenge_participants(owner_id,id)
);
CREATE INDEX challenge_source_history ON challenge_source_credits(owner_id,participant_id,source_time DESC,source_key DESC);
CREATE INDEX challenge_reward_sources ON reward_entitlements(owner_id,source_kind,first_awarded_at) WHERE active;
ALTER TABLE milestones ADD COLUMN challenge_first_qualified_at timestamptz;
ALTER TABLE milestones ADD COLUMN challenge_completed boolean NOT NULL DEFAULT false;
ALTER TABLE milestones ADD COLUMN challenge_legacy_completed boolean NOT NULL DEFAULT false;
-- Existing completed stages have no trusted first-qualification timestamp for
-- this feature. Preserve their history and do not manufacture a new event.
WITH RECURSIVE tree AS (
 SELECT l.owner_id,l.goal_id,l.quest_id id,l.milestone_id,l.required AND COALESCE((q.payload->>'required')::boolean,true) mandatory FROM goal_quests l JOIN quests q ON q.owner_id=l.owner_id AND q.id=l.quest_id
 UNION ALL SELECT t.owner_id,t.goal_id,c.id,t.milestone_id,t.mandatory AND COALESCE((c.payload->>'required')::boolean,true) FROM tree t JOIN quests c ON c.owner_id=t.owner_id AND c.parent_id=t.id
), leaves AS (
 SELECT DISTINCT ON(t.owner_id,t.goal_id,t.id) t.*,q.status FROM tree t JOIN quests q ON q.owner_id=t.owner_id AND q.id=t.id WHERE NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=t.owner_id AND c.parent_id=t.id) ORDER BY t.owner_id,t.goal_id,t.id
), parts AS (
 SELECT m.owner_id,m.id,CASE WHEN EXISTS(SELECT 1 FROM goal_quests l WHERE l.owner_id=m.owner_id AND l.goal_id=m.goal_id AND l.milestone_id=m.id)
 THEN EXISTS(SELECT 1 FROM leaves l WHERE l.owner_id=m.owner_id AND l.goal_id=m.goal_id AND l.milestone_id=m.id AND l.mandatory) AND NOT EXISTS(SELECT 1 FROM leaves l WHERE l.owner_id=m.owner_id AND l.goal_id=m.goal_id AND l.milestone_id=m.id AND l.mandatory AND l.status<>'completed') ELSE m.completed END done FROM milestones m
)
UPDATE milestones m SET challenge_completed=p.done,challenge_legacy_completed=p.done FROM parts p WHERE p.owner_id=m.owner_id AND p.id=m.id;
-- +goose StatementBegin
CREATE FUNCTION protect_stage_qualification() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.challenge_legacy_completed IS DISTINCT FROM NEW.challenge_legacy_completed OR (OLD.challenge_first_qualified_at IS NOT NULL AND OLD.challenge_first_qualified_at IS DISTINCT FROM NEW.challenge_first_qualified_at) THEN RAISE EXCEPTION 'first stage qualification is immutable'; END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER stage_qualification_guard BEFORE UPDATE ON milestones FOR EACH ROW EXECUTE FUNCTION protect_stage_qualification();
-- +goose Down
DROP TRIGGER stage_qualification_guard ON milestones;
DROP FUNCTION protect_stage_qualification();
ALTER TABLE milestones DROP COLUMN challenge_first_qualified_at;
ALTER TABLE milestones DROP COLUMN challenge_completed;
ALTER TABLE milestones DROP COLUMN challenge_legacy_completed;
DROP INDEX challenge_reward_sources;
DROP TABLE challenge_source_credits;
