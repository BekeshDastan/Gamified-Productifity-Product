package goals

import (
	"context"
	"github.com/jackc/pgx/v5"
)

type BattleStage struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Required  bool    `json:"required"`
	Progress  float64 `json:"progress"`
	SourceURL string  `json:"source_url"`
}

func (s *Service) BattleSourceInTx(ctx context.Context, tx pgx.Tx, owner, id string) (Goal, []BattleStage, error) {
	g, e := s.get(ctx, tx, owner, id)
	out := []BattleStage{}
	if e != nil {
		return g, out, e
	}
	g.Progress, e = s.calculate(ctx, tx, owner, g)
	if e != nil {
		return g, out, e
	}
	plan, e := s.plan(ctx, tx, owner, id)
	if e != nil {
		return g, out, e
	}
	if g.ProgressType == "milestones" {
		for _, m := range plan.Milestones {
			var linked bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM goal_quests WHERE owner_id=$1 AND goal_id=$2 AND milestone_id=$3)", owner, id, m.ID).Scan(&linked); e != nil {
				return g, out, e
			}
			progress := float64(0)
			if linked {
				e = tx.QueryRow(ctx, leavesSQL+"SELECT COALESCE(100*sum(weight) FILTER(WHERE status='completed')/NULLIF(sum(weight),0),0)::float8 FROM leaves WHERE milestone_id=$3 AND mandatory", owner, id, m.ID).Scan(&progress)
				if e != nil {
					return g, out, e
				}
			} else if m.Completed {
				progress = 100
			}
			out = append(out, BattleStage{m.ID, m.Title, m.Required, progress, "/app/goals/" + id})
		}
	} else if g.ProgressType == "quests" {
		rows, e := tx.Query(ctx, leavesSQL+"SELECT l.id::text,q.payload->>'title',l.mandatory,CASE WHEN l.status='completed' THEN 100 ELSE 0 END FROM leaves l JOIN quests q ON q.owner_id=$1 AND q.id=l.id ORDER BY l.id", owner, id)
		if e != nil {
			return g, out, e
		}
		for rows.Next() {
			var stage BattleStage
			if e = rows.Scan(&stage.ID, &stage.Title, &stage.Required, &stage.Progress); e != nil {
				rows.Close()
				return g, out, e
			}
			stage.SourceURL = "/app/quests/" + stage.ID
			out = append(out, stage)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return g, out, e
		}
	} else {
		out = append(out, BattleStage{g.ID, g.Title, true, g.Progress, "/app/goals/" + id})
	}
	return g, out, nil
}
