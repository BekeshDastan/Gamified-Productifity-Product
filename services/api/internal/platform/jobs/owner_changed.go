package jobs

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// OwnerChanged emits derived-profile work in the same transaction as a source edit.
func OwnerChanged(ctx context.Context, tx pgx.Tx, owner string, clock Clock) error {
	_, e := tx.Exec(ctx, "INSERT INTO outbox_events(id,owner_id,kind,payload,occurred_at) VALUES(gen_random_uuid(),$1,'character.changed','{}',$2)", owner, clock.Now())
	return e
}
