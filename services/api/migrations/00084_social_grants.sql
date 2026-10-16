-- +goose Up
CREATE TABLE social_grants (
 owner_id uuid NOT NULL REFERENCES users(id), recipient_id uuid NOT NULL REFERENCES users(id),
 metric text NOT NULL CHECK(metric IN('regularity','evening','challenge')),
 scope text NOT NULL DEFAULT '', created_at timestamptz NOT NULL,
 PRIMARY KEY(owner_id,recipient_id,metric,scope), CHECK(owner_id<>recipient_id),
 CHECK((metric='challenge' AND scope<>'') OR(metric<>'challenge' AND scope=''))
);
-- +goose Down
DROP TABLE social_grants;
