-- +goose Up
CREATE TABLE social_reports (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner_id uuid NOT NULL REFERENCES users(id),target_id uuid NOT NULL REFERENCES users(id),
 post_id uuid REFERENCES social_posts(id),reason text NOT NULL CHECK(reason IN('spam','abuse','privacy','other')),
 resolved boolean NOT NULL DEFAULT false,created_at timestamptz NOT NULL
);
-- +goose Down
DROP TABLE social_reports;
