package goals

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// Stage qualification follows the same mandatory-leaf calculation as the goal.
// Its first timestamp never moves when a source is reopened or restored.
func (s *Service) observeStages(ctx context.Context, tx pgx.Tx, owner, goal string) error {
	_, e := tx.Exec(ctx, leavesSQL+`, parts AS (
 SELECT m.id,CASE WHEN EXISTS(SELECT 1 FROM goal_quests q WHERE q.owner_id=$1 AND q.goal_id=$2 AND q.milestone_id=m.id)
 THEN EXISTS(SELECT 1 FROM leaves l WHERE l.milestone_id=m.id AND l.mandatory) AND NOT EXISTS(SELECT 1 FROM leaves l WHERE l.milestone_id=m.id AND l.mandatory AND l.status<>'completed') ELSE m.completed END done
 FROM milestones m WHERE m.owner_id=$1 AND m.goal_id=$2)
 UPDATE milestones m SET challenge_first_qualified_at=CASE WHEN p.done AND NOT m.challenge_completed AND NOT m.challenge_legacy_completed AND m.challenge_first_qualified_at IS NULL THEN $3 ELSE m.challenge_first_qualified_at END,challenge_completed=p.done FROM parts p WHERE m.owner_id=$1 AND m.goal_id=$2 AND m.id=p.id`, owner, goal, s.clock.Now().UTC())
	return e
}
