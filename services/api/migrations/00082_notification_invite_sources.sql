-- +goose Up
-- Trusted source receipt; stage 21 supplies invitation lifecycle and explicit recipient consent.
CREATE TABLE notification_invite_sources(
 recipient_id uuid NOT NULL REFERENCES users(id),id uuid NOT NULL,revision bigint NOT NULL CHECK(revision>0),
 active boolean NOT NULL DEFAULT true,expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL,
 PRIMARY KEY(recipient_id,id)
);
-- +goose Down
DROP TABLE notification_invite_sources;
