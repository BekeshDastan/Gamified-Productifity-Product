-- +goose Up
CREATE TABLE notification_preferences(owner_id uuid PRIMARY KEY REFERENCES users(id),payload jsonb NOT NULL,version bigint NOT NULL DEFAULT 1 CHECK(version>0),updated_at timestamptz NOT NULL);
CREATE TABLE notification_preference_events(owner_id uuid NOT NULL REFERENCES users(id),version bigint NOT NULL,snapshot jsonb NOT NULL,created_at timestamptz NOT NULL,PRIMARY KEY(owner_id,version));
CREATE TRIGGER notification_preferences_audit_immutable BEFORE UPDATE OR DELETE ON notification_preference_events FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
ALTER TABLE notifications ADD COLUMN muted boolean NOT NULL DEFAULT false;
DROP INDEX notifications_owner_unread;
CREATE INDEX notifications_owner_unread ON notifications(owner_id) WHERE read_at IS NULL AND active AND NOT muted;
-- +goose Down
DROP INDEX notifications_owner_unread;
ALTER TABLE notifications DROP COLUMN muted;
CREATE INDEX notifications_owner_unread ON notifications(owner_id) WHERE read_at IS NULL AND active;
DROP TABLE notification_preference_events;
DROP TABLE notification_preferences;
