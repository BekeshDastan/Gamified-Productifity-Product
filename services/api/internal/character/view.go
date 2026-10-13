package character

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// ViewInTx shares the caller's owner/wallet lock and transaction.
func (s *Service) ViewInTx(ctx context.Context, tx pgx.Tx, owner string) (Profile, error) {
	return s.view(ctx, tx, owner)
}
