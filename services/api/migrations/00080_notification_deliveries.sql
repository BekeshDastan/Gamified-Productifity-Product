-- +goose Up
CREATE TABLE notification_deliveries(
 owner_id uuid NOT NULL REFERENCES users(id),id uuid NOT NULL DEFAULT gen_random_uuid(),notification_id uuid NOT NULL,
 channel text NOT NULL CHECK(channel IN('email','push')),subscription_id uuid,
 state text NOT NULL CHECK(state IN('queued','retry_wait','sending','provider_accepted','delivery_unknown','failed','cancelled')),
 version bigint NOT NULL DEFAULT 1,attempts integer NOT NULL DEFAULT 0,auto_attempts integer NOT NULL DEFAULT 0,
 next_run_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,last_attempt_id uuid,last_error_code text NOT NULL DEFAULT '',
 provider_accepted_at timestamptz,unknown_at timestamptz,created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,id),FOREIGN KEY(owner_id,notification_id) REFERENCES notifications(owner_id,id)
);
CREATE UNIQUE INDEX notification_delivery_once ON notification_deliveries(owner_id,notification_id,channel,COALESCE(subscription_id,'00000000-0000-0000-0000-000000000000'::uuid));
CREATE INDEX notification_delivery_pending ON notification_deliveries(owner_id,next_run_at) WHERE state IN('queued','retry_wait','sending');
CREATE TABLE notification_delivery_attempts(
 owner_id uuid NOT NULL,delivery_id uuid NOT NULL,attempt integer NOT NULL,attempt_id uuid NOT NULL UNIQUE,
 channel text NOT NULL,state text NOT NULL CHECK(state IN('sending','provider_accepted','delivery_unknown','failed')),
 started_at timestamptz NOT NULL,finished_at timestamptz,error_code text NOT NULL DEFAULT '',
 PRIMARY KEY(owner_id,delivery_id,attempt),FOREIGN KEY(owner_id,delivery_id) REFERENCES notification_deliveries(owner_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION protect_notification_attempt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR OLD.state<>'sending' THEN RAISE EXCEPTION 'delivery evidence is immutable'; END IF;
 IF (OLD.owner_id,OLD.delivery_id,OLD.attempt,OLD.attempt_id,OLD.channel,OLD.started_at) IS DISTINCT FROM (NEW.owner_id,NEW.delivery_id,NEW.attempt,NEW.attempt_id,NEW.channel,NEW.started_at) THEN RAISE EXCEPTION 'delivery attempt identity is immutable'; END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER notification_attempt_guard BEFORE UPDATE OR DELETE ON notification_delivery_attempts FOR EACH ROW EXECUTE FUNCTION protect_notification_attempt();
-- +goose Down
DROP TABLE notification_delivery_attempts;
DROP FUNCTION protect_notification_attempt();
DROP TABLE notification_deliveries;
