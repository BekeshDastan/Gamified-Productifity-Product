-- name: RegisterUser :one
INSERT INTO users (email, password_hash, created_at, updated_at)
VALUES ($1, $2, $3, $3)
RETURNING id::text AS id, email, role, email_verified_at, created_at;

-- name: FindUserCredentials :one
SELECT id::text AS id, email, password_hash, role, email_verified_at, disabled_at
FROM users WHERE email = $1;
