-- +goose Up
ALTER TABLE auth_one_time_tokens DROP CONSTRAINT auth_one_time_kind;
ALTER TABLE auth_one_time_tokens ADD CONSTRAINT auth_one_time_kind CHECK(kind IN ('verify_email','reset_password'));

-- +goose Down
DELETE FROM auth_one_time_tokens WHERE kind='reset_password';
ALTER TABLE auth_one_time_tokens DROP CONSTRAINT auth_one_time_kind;
ALTER TABLE auth_one_time_tokens ADD CONSTRAINT auth_one_time_kind CHECK(kind IN ('verify_email'));
