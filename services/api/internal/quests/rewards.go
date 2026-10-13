package quests

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/jackc/pgx/v5"
)

func (s *Service) syncReward(ctx context.Context, tx pgx.Tx, id, kind string, q *Quest) error {
	sum, e := summary(ctx, tx, id, q.ID)
	if e != nil {
		return e
	}
	reward, e := s.progression.Sync(ctx, tx, id, progression.Source{Kind: "task", Key: q.ID, Difficulty: q.Difficulty, AreaID: q.AreaID, Complete: q.Status == "completed", Eligible: !sum.Container, AllowFirst: kind == "complete"})
	if e == nil && reward != nil && (kind == "complete" || kind == "reopen" || reward.Kind != "unchanged") {
		q.Reward = reward
	}
	return e
}
func (s *Service) reconcileRollupReward(ctx context.Context, tx pgx.Tx, id string, q *Quest) error {
	before := *q
	if e := s.syncReward(ctx, tx, id, "rollup_reward", q); e != nil {
		return e
	}
	if q.Reward == nil {
		return nil
	}
	reward := q.Reward
	fresh, e := scan(tx.QueryRow(ctx, `UPDATE quests SET version=version+1,updated_at=$3 WHERE owner_id=$1 AND id=$2 RETURNING `+columns, id, q.ID, s.clock.Now().UTC()))
	if e != nil {
		return e
	}
	*q = fresh
	q.Reward = reward
	return s.event(ctx, tx, id, "reward_reconciled", "Пересчёт награды после изменения структуры", &before, q)
}
