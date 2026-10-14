package goals

import (
	"context"
	"github.com/jackc/pgx/v5"
)

type NavigatorFact struct {
	ID          string
	Version     int64
	Overdue     bool
	Progress    float64
	NextActions int
}

func (s *Service) NavigatorFactsInTx(ctx context.Context, tx pgx.Tx, owner string) ([]NavigatorFact, error) {
	rows, e := tx.Query(ctx, `SELECT id::text FROM goals WHERE owner_id=$1 AND state='active' AND archived_at IS NULL ORDER BY id`, owner)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	out := []NavigatorFact{}
	for _, id := range ids {
		g, e := s.get(ctx, tx, owner, id)
		if e != nil {
			return nil, e
		}
		v := NavigatorFact{ID: id, Version: g.Version, Overdue: g.Overdue, Progress: g.Progress}
		e = tx.QueryRow(ctx, `SELECT count(*) FROM goal_quests l JOIN quests q ON q.owner_id=l.owner_id AND q.id=l.quest_id WHERE l.owner_id=$1 AND l.goal_id=$2 AND q.archived_at IS NULL AND q.status IN('todo','backlog','in_progress','postponed')`, owner, id).Scan(&v.NextActions)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
