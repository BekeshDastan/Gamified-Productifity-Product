package character

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/jackc/pgx/v5"
)

func (s *Service) facts(ctx context.Context, tx pgx.Tx, owner string, advance ...bool) (Facts, []habits.Evidence, bool, error) {
	f := Facts{Values: map[Metric]int64{}, Available: map[Metric]bool{}}
	queries := map[Metric]string{
		CompletedSeasons: "SELECT count(*) FROM personal_seasons WHERE owner_id=$1 AND state='completed' AND activated_at IS NOT NULL",
		Tasks:            "SELECT count(*) FROM quests q WHERE owner_id=$1 AND status='completed' AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id)",
		ImportantTasks:   "SELECT count(*) FROM quests q WHERE owner_id=$1 AND status='completed' AND payload->>'priority' ='high' AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id)",
		GoalsCreated:     "SELECT count(*) FROM goals WHERE owner_id=$1", GoalsAchieved: "SELECT count(*) FROM goals WHERE owner_id=$1 AND state='achieved'", HabitsCreated: "SELECT count(*) FROM habits WHERE owner_id=$1", ConfirmedPlans: "SELECT count(*) FROM day_plans WHERE owner_id=$1 AND confirmed_at IS NOT NULL", AreasCreated: "SELECT count(*) FROM life_areas WHERE owner_id=$1"}
	for m, q := range queries {
		var n int64
		if e := tx.QueryRow(ctx, q, owner).Scan(&n); e != nil {
			return f, nil, false, e
		}
		f.Values[m] = n
		f.Available[m] = true
	}
	balance, e := s.progress.Balance(ctx, tx, owner)
	if e != nil {
		return f, nil, false, e
	}
	var validSessions int64
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM focus_sessions f WHERE f.owner_id=$1 AND f.state='finished' AND f.result='progress' AND (SELECT COALESCE(sum(floor(extract(epoch FROM(ended_at-started_at)))),0) FROM focus_segments WHERE owner_id=f.owner_id AND session_id=f.id AND kind='work' AND ended_at IS NOT NULL)>=300`, owner).Scan(&validSessions); e != nil {
		return f, nil, false, e
	}
	f.Values[FocusSessions] = validSessions
	f.Available[FocusSessions] = true
	f.Values[PlayerLevel] = balance.Level.Number
	f.Available[PlayerLevel] = true
	evidence, ready, e := habits.NewService(s.pool, s.clock).EvidenceInTx(ctx, tx, owner, advance...)
	if e != nil {
		return f, nil, false, e
	}
	for _, h := range evidence {
		f.Values[SuccessfulPeriods] += int64(h.Stats.Successful)
		f.Values[BestStreak] = max(f.Values[BestStreak], int64(h.Stats.Best))
		awaiting := false
		for _, p := range h.Periods {
			if p.Excluded {
				continue
			}
			if p.Amount >= p.Target {
				if awaiting {
					f.Values[Comebacks]++
					awaiting = false
				}
			} else if !s.clock.Now().Before(p.End) {
				awaiting = true
			}
		}
	}
	for _, m := range []Metric{SuccessfulPeriods, BestStreak, Comebacks} {
		f.Available[m] = ready
	}
	return f, evidence, ready, nil
}
