-- +goose Up
CREATE TABLE notification_push_subscriptions(
 owner_id uuid NOT NULL REFERENCES users(id),id uuid NOT NULL DEFAULT gen_random_uuid(),
 endpoint text NOT NULL,endpoint_hash text NOT NULL,p256dh text NOT NULL,auth text NOT NULL,
 expires_at timestamptz NOT NULL,revoked_at timestamptz,version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,id)
);
CREATE UNIQUE INDEX notification_push_active_endpoint ON notification_push_subscriptions(endpoint_hash) WHERE revoked_at IS NULL;
ALTER TABLE notification_deliveries ADD FOREIGN KEY(owner_id,subscription_id) REFERENCES notification_push_subscriptions(owner_id,id);
-- +goose Down
ALTER TABLE notification_deliveries DROP CONSTRAINT notification_deliveries_owner_id_subscription_id_fkey;
DROP TABLE notification_push_subscriptions;
