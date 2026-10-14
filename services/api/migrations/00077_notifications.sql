-- +goose Up
CREATE TABLE notifications(
 owner_id uuid NOT NULL REFERENCES users(id),id uuid NOT NULL DEFAULT gen_random_uuid(),
 kind text NOT NULL CHECK(kind IN('deadline','habit','achievement','invite','report','focus_result')),
 source_kind text NOT NULL,source_id text NOT NULL,source_revision bigint NOT NULL CHECK(source_revision>0),
 event_key text NOT NULL,source_url text NOT NULL,reminder_id uuid,
 title text NOT NULL,message text NOT NULL,created_at timestamptz NOT NULL,read_at timestamptz,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),active boolean NOT NULL DEFAULT true,
 PRIMARY KEY(owner_id,id),UNIQUE(owner_id,event_key,source_revision)
);
CREATE INDEX notifications_owner_history ON notifications(owner_id,created_at DESC,id DESC);
CREATE INDEX notifications_owner_unread ON notifications(owner_id) WHERE read_at IS NULL AND active;
-- +goose StatementBegin
CREATE FUNCTION protect_notification_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'notification history is immutable'; END IF;
 IF (OLD.owner_id,OLD.id,OLD.kind,OLD.source_kind,OLD.source_id,OLD.source_revision,OLD.event_key,OLD.source_url,OLD.reminder_id,OLD.title,OLD.message,OLD.created_at) IS DISTINCT FROM (NEW.owner_id,NEW.id,NEW.kind,NEW.source_kind,NEW.source_id,NEW.source_revision,NEW.event_key,NEW.source_url,NEW.reminder_id,NEW.title,NEW.message,NEW.created_at) THEN RAISE EXCEPTION 'notification source is immutable'; END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER notification_source_immutable BEFORE UPDATE OR DELETE ON notifications FOR EACH ROW EXECUTE FUNCTION protect_notification_source();
-- +goose Down
DROP TABLE notifications;
DROP FUNCTION protect_notification_source();
