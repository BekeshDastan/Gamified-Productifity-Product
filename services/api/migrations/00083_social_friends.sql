-- +goose Up
CREATE TABLE social_profiles (
 owner_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 invite_code uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
 nudges boolean NOT NULL DEFAULT false,
 partner_id uuid REFERENCES users(id),
 version bigint NOT NULL DEFAULT 1
);
CREATE TABLE social_friends (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 sender uuid NOT NULL REFERENCES users(id), recipient uuid NOT NULL REFERENCES users(id),
 state text NOT NULL CHECK(state IN ('pending','accepted','rejected','removed')),
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
 CHECK(sender<>recipient)
);
CREATE UNIQUE INDEX social_friend_pair ON social_friends(LEAST(sender,recipient),GREATEST(sender,recipient)) WHERE state IN ('pending','accepted');
CREATE INDEX social_friend_inbox ON social_friends(recipient,created_at);
CREATE TABLE social_blocks (
 owner_id uuid NOT NULL REFERENCES users(id), target_id uuid NOT NULL REFERENCES users(id),
 created_at timestamptz NOT NULL, PRIMARY KEY(owner_id,target_id), CHECK(owner_id<>target_id)
);
-- +goose Down
DROP TABLE social_blocks,social_friends,social_profiles;
