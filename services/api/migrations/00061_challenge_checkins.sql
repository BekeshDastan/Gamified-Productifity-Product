-- +goose Up
CREATE UNIQUE INDEX attachments_owner_identity ON attachments(owner_id,id);
CREATE TABLE challenge_checkins (
 owner_id uuid NOT NULL, id uuid NOT NULL, participant_id uuid NOT NULL,
 request jsonb NOT NULL, local_date date NOT NULL, unit text NOT NULL,
 initial_amount bigint NOT NULL CHECK(initial_amount>0), amount bigint NOT NULL CHECK(amount BETWEEN 0 AND initial_amount),
 note text NOT NULL DEFAULT '', version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,id),
 FOREIGN KEY(owner_id,participant_id) REFERENCES challenge_participants(owner_id,id)
);
CREATE INDEX challenge_checkin_page ON challenge_checkins(owner_id,participant_id,created_at DESC,id DESC);
CREATE TABLE challenge_checkin_events (
 owner_id uuid NOT NULL, checkin_id uuid NOT NULL, version bigint NOT NULL,
 snapshot jsonb NOT NULL, created_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,checkin_id,version),
 FOREIGN KEY(owner_id,checkin_id) REFERENCES challenge_checkins(owner_id,id)
);
CREATE TABLE challenge_evidence (
 owner_id uuid NOT NULL, checkin_id uuid NOT NULL, attachment_id uuid NOT NULL,
 visibility text NOT NULL DEFAULT 'private' CHECK(visibility IN('private','participants')),
 consent_at timestamptz, version bigint NOT NULL DEFAULT 1, updated_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,checkin_id,attachment_id),
 FOREIGN KEY(owner_id,checkin_id) REFERENCES challenge_checkins(owner_id,id),
 FOREIGN KEY(owner_id,attachment_id) REFERENCES attachments(owner_id,id),
 CHECK((visibility='private' AND consent_at IS NULL) OR (visibility='participants' AND consent_at IS NOT NULL))
);
-- +goose StatementBegin
CREATE FUNCTION protect_challenge_checkin() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(OLD.owner_id,OLD.id,OLD.participant_id,OLD.request,OLD.local_date,OLD.unit,OLD.initial_amount,OLD.created_at) IS DISTINCT FROM ROW(NEW.owner_id,NEW.id,NEW.participant_id,NEW.request,NEW.local_date,NEW.unit,NEW.initial_amount,NEW.created_at) THEN RAISE EXCEPTION 'original check-in is immutable'; END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER challenge_checkin_guard BEFORE UPDATE ON challenge_checkins FOR EACH ROW EXECUTE FUNCTION protect_challenge_checkin();
CREATE TRIGGER challenge_checkin_events_immutable BEFORE UPDATE OR DELETE ON challenge_checkin_events FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
CREATE TABLE challenge_evidence_events (
 owner_id uuid NOT NULL, checkin_id uuid NOT NULL, attachment_id uuid NOT NULL, version bigint NOT NULL,
 snapshot jsonb NOT NULL, created_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,checkin_id,attachment_id,version),
 FOREIGN KEY(owner_id,checkin_id,attachment_id) REFERENCES challenge_evidence(owner_id,checkin_id,attachment_id)
);
CREATE TRIGGER challenge_evidence_events_immutable BEFORE UPDATE OR DELETE ON challenge_evidence_events FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE challenge_evidence_events;
DROP TABLE challenge_evidence;
DROP TABLE challenge_checkin_events;
DROP TABLE challenge_checkins;
DROP FUNCTION protect_challenge_checkin();
DROP INDEX attachments_owner_identity;
