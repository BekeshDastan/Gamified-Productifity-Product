package challenges

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// ProgressInTx shares source validation and correction handling with the normal
// challenge API. The caller holds the common owner lock.
func (s *Service) ProgressInTx(ctx context.Context, tx pgx.Tx, owner, id string) (Member, error) {
	return s.currentProgress(ctx, tx, owner, id)
}
func (s *Service) CreateInTx(ctx context.Context, tx pgx.Tx, owner string, in CreateInput) (Run, error) {
	return s.createRun(ctx, tx, owner, in)
}
func (s *Service) JoinInTx(ctx context.Context, tx pgx.Tx, owner, id string, in JoinInput) (Member, error) {
	return s.join(ctx, tx, owner, id, in)
}
