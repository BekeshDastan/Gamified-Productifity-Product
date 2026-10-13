-- +goose Up
CREATE TABLE challenge_participants (
 owner_id uuid NOT NULL REFERENCES users(id), id uuid NOT NULL DEFAULT gen_random_uuid(),
 run_owner_id uuid NOT NULL, run_id uuid NOT NULL, rules_hash text NOT NULL,
 state text NOT NULL CHECK(state IN('joined','left','finished')), version bigint NOT NULL DEFAULT 1,
 first_joined_at timestamptz NOT NULL, left_at timestamptz, leave_reason text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL, progress jsonb, computed_at timestamptz,
 result text CHECK(result IN('achieved','not_achieved','left')), final_snapshot jsonb, closed_at timestamptz,
 PRIMARY KEY(owner_id,id), UNIQUE(run_owner_id,run_id,owner_id),
 FOREIGN KEY(run_owner_id,run_id) REFERENCES challenge_runs(owner_id,id)
);
CREATE TABLE challenge_membership_windows (
 owner_id uuid NOT NULL, participant_id uuid NOT NULL, sequence integer NOT NULL,
 joined_at timestamptz NOT NULL, left_at timestamptz CHECK(left_at>=joined_at),
 PRIMARY KEY(owner_id,participant_id,sequence),
 FOREIGN KEY(owner_id,participant_id) REFERENCES challenge_participants(owner_id,id)
);
CREATE UNIQUE INDEX challenge_membership_open ON challenge_membership_windows(owner_id,participant_id) WHERE left_at IS NULL;
CREATE TABLE challenge_participant_events (
 owner_id uuid NOT NULL, participant_id uuid NOT NULL, version bigint NOT NULL,
 kind text NOT NULL, snapshot jsonb NOT NULL, created_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,participant_id,version), FOREIGN KEY(owner_id,participant_id) REFERENCES challenge_participants(owner_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION protect_challenge_membership() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'challenge membership history is retained'; END IF;
 IF TG_TABLE_NAME='challenge_participant_events' THEN RAISE EXCEPTION 'participant events are immutable'; END IF;
 IF TG_TABLE_NAME='challenge_membership_windows' THEN
  IF (OLD.owner_id,OLD.participant_id,OLD.sequence,OLD.joined_at) IS DISTINCT FROM (NEW.owner_id,NEW.participant_id,NEW.sequence,NEW.joined_at) OR OLD.left_at IS NOT NULL THEN RAISE EXCEPTION 'closed membership windows are immutable'; END IF;
 ELSE
  IF (OLD.owner_id,OLD.id,OLD.run_owner_id,OLD.run_id,OLD.rules_hash,OLD.first_joined_at) IS DISTINCT FROM (NEW.owner_id,NEW.id,NEW.run_owner_id,NEW.run_id,NEW.rules_hash,NEW.first_joined_at) THEN RAISE EXCEPTION 'original membership is immutable'; END IF;
  IF OLD.final_snapshot IS NOT NULL AND OLD.final_snapshot IS DISTINCT FROM NEW.final_snapshot THEN RAISE EXCEPTION 'initial participant result is immutable'; END IF;
  IF OLD.state='finished' AND NEW.state<>'finished' THEN RAISE EXCEPTION 'finished member cannot rejoin'; END IF;
 END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER challenge_participant_guard BEFORE UPDATE OR DELETE ON challenge_participants FOR EACH ROW EXECUTE FUNCTION protect_challenge_membership();
CREATE TRIGGER challenge_windows_guard BEFORE UPDATE OR DELETE ON challenge_membership_windows FOR EACH ROW EXECUTE FUNCTION protect_challenge_membership();
CREATE TRIGGER challenge_participant_events_guard BEFORE UPDATE OR DELETE ON challenge_participant_events FOR EACH ROW EXECUTE FUNCTION protect_challenge_membership();
-- +goose Down
DROP TABLE challenge_participant_events;
DROP TABLE challenge_membership_windows;
DROP TABLE challenge_participants;
DROP FUNCTION protect_challenge_membership();
