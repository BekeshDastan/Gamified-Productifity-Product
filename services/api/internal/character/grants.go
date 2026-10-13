package character

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

func (s *Service) GrantPermanentInTx(ctx context.Context, tx pgx.Tx, owner, item, kind, key string) error {
	if _, ok := itemByID(item); !ok || (kind != "purchase" && kind != "mystery" && kind != "boss") || len(key) < 1 || len(key) > 200 {
		return apiError(422, "VALIDATION_ERROR", "Недопустимая выдача предмета")
	}
	if e := s.ensure(ctx, tx, owner); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, "INSERT INTO cosmetic_permanent_grants(owner_id,item_id,source_kind,source_key,granted_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING", owner, item, kind, key, s.clock.Now())
	if e != nil {
		return e
	}
	var matches bool
	if e = tx.QueryRow(ctx, "SELECT item_id=$4 FROM cosmetic_permanent_grants WHERE owner_id=$1 AND source_kind=$2 AND source_key=$3", owner, kind, key, item).Scan(&matches); e != nil {
		return e
	}
	if !matches {
		return apiError(409, "IDEMPOTENCY_MISMATCH", "Эта выдача уже содержит другой предмет")
	}
	if e = s.EvaluateInTx(ctx, tx, owner); e != nil {
		return e
	}
	return jobs.OwnerChanged(ctx, tx, owner, s.clock)
}
