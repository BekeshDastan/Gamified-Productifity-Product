package jobs

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// OwnerChanged emits derived-profile work in the same transaction as a source edit.
func OwnerChanged(ctx context.Context, tx pgx.Tx, owner string, clock Clock) error {
	if _, e := tx.Exec(ctx, "INSERT INTO analytics_projection_state(owner_id) VALUES($1) ON CONFLICT(owner_id) DO UPDATE SET source_revision=analytics_projection_state.source_revision+1", owner); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, "INSERT INTO outbox_events(id,owner_id,kind,payload,occurred_at) VALUES(gen_random_uuid(),$1,'character.changed','{}',$2),(gen_random_uuid(),$1,'challenges.changed','{}',$2),(gen_random_uuid(),$1,'analytics.changed','{}',$2),(gen_random_uuid(),$1,'notifications.changed','{}',$2)", owner, clock.Now())
	return e
}
