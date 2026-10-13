package quests

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/jackc/pgx/v5"
)

func (s *Service) awardCompletion(ctx context.Context, tx pgx.Tx, id string, q *Quest) error {
	sum, e := summary(ctx, tx, id, q.ID)
	if e != nil {
		return e
	}
	if sum.Container {
		return nil
	}
	q.Reward, e = s.progression.Award(ctx, tx, id, progression.Source{Kind: "task", Key: q.ID, Difficulty: q.Difficulty, AreaID: q.AreaID, Complete: q.Status == "completed", Eligible: true, AllowFirst: true})
	return e
}
