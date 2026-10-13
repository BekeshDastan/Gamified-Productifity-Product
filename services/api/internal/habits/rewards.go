package habits

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Service) rewardedView(ctx context.Context, tx pgx.Tx, owner string, h Habit, key string) (Habit, error) {
	out, e := s.view(ctx, tx, owner, h.ID)
	if e != nil || s.progression == nil {
		return out, e
	}
	var end time.Time
	var amount, target float64
	var excluded bool
	e = tx.QueryRow(ctx, `SELECT p.ends_at,p.target,p.excluded,COALESCE((SELECT sum(l.amount) FROM habit_logs l WHERE l.owner_id=p.owner_id AND l.period_id=p.id),0)::double precision FROM habit_periods p WHERE p.owner_id=$1 AND p.habit_id=$2 AND p.id=$3`, owner, h.ID, key).Scan(&end, &target, &excluded, &amount)
	if e != nil {
		return out, e
	}
	if amount < target || excluded {
		return out, nil
	}
	source := progression.Source{Kind: "habit", Key: key, AreaID: h.AreaID, Complete: true, Eligible: true, AllowFirst: !s.clock.Now().After(end.Add(24 * time.Hour))}
	out.Reward, e = s.progression.Award(ctx, tx, owner, source)
	if e != nil {
		return out, e
	}
	if out.Reward == nil && !source.AllowFirst {
		out.Reward, e = s.progression.ExplainNoReward(ctx, tx, owner, source, "История сохранена. Новая награда недоступна после конца периода + 24 часа.")
	}
	return out, e
}
