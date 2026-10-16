-- +goose Up
CREATE TABLE social_reviews (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), checkin_id uuid NOT NULL UNIQUE REFERENCES social_checkins(id),
 owner_id uuid NOT NULL REFERENCES users(id), reviewer_id uuid NOT NULL REFERENCES users(id),
 checkin_version bigint NOT NULL,attachment_id uuid, score_state text NOT NULL DEFAULT 'pending' CHECK(score_state IN('pending','accepted','rejected','disputed')),
 state text NOT NULL CHECK(state IN('pending','accepted','rejected','disputed','withdrawn')),
 version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,
 CHECK(owner_id<>reviewer_id),FOREIGN KEY(owner_id,attachment_id) REFERENCES attachments(owner_id,id)
);
CREATE TABLE social_review_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,review_id uuid NOT NULL REFERENCES social_reviews(id),
 actor_id uuid NOT NULL REFERENCES users(id),state text NOT NULL,version bigint NOT NULL,created_at timestamptz NOT NULL
);
CREATE TRIGGER social_review_events_immutable BEFORE UPDATE OR DELETE ON social_review_events FOR EACH ROW EXECUTE FUNCTION reject_economy_rewrite();
-- +goose Down
DROP TABLE social_review_events,social_reviews;
