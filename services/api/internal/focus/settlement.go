package focus

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

// SettleDueInTx reuses the same timer boundaries as a foreground read. The
// caller holds the owner lock; no browser or active tab is required.
func (s *Service) SettleDueInTx(ctx context.Context, tx pgx.Tx, owner string) error {
	var id string
	e := tx.QueryRow(ctx, `SELECT id::text FROM focus_sessions WHERE owner_id=$1 AND state IN('running','paused')`, owner).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	_, e = s.get(ctx, tx, owner, id)
	return e
}
