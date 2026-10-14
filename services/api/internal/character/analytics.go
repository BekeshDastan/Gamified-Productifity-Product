package character

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// RecordsInTx reads canonical personal records under the common owner lock.
func (s *Service) RecordsInTx(ctx context.Context, tx pgx.Tx, owner string) ([]Record, error) {
	if e := s.progress.LockOwner(ctx, tx, owner); e != nil {
		return nil, e
	}
	return s.records(ctx, tx, owner)
}
