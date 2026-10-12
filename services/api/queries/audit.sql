-- name: RecordAuditEvent :one
INSERT INTO audit_events (actor_id, action, entity_type, entity_id, metadata)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ListActorAuditEvents :many
SELECT * FROM audit_events WHERE actor_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2;
