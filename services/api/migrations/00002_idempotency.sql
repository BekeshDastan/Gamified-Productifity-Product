-- +goose Up
CREATE TABLE idempotency_records (
    user_id uuid NOT NULL,
    method text NOT NULL CHECK (method IN ('POST', 'PUT', 'PATCH', 'DELETE')),
    route_key text NOT NULL CHECK (char_length(route_key) BETWEEN 1 AND 512),
    idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 16 AND 128),
    body_hash bytea NOT NULL CHECK (octet_length(body_hash) = 32),
    status_code integer CHECK (status_code BETWEEN 200 AND 299),
    response_body bytea,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at >= created_at + interval '24 hours'),
    PRIMARY KEY (user_id, method, route_key, idempotency_key),
    CHECK ((status_code IS NULL) = (response_body IS NULL))
);
CREATE INDEX idempotency_expiry ON idempotency_records (expires_at);
-- +goose Down
DROP TABLE idempotency_records;
