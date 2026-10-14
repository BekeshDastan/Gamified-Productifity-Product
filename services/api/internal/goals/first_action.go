package goals

import (
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/jackc/pgx/v5"
	"net/http"
)

type FirstActionResult struct {
	Goal Goal         `json:"goal"`
	Task quests.Quest `json:"task"`
}

// AddFirstActionInTx creates and attaches the reviewed new root in one owner transaction.
func (s *Service) AddFirstActionInTx(r *http.Request, tx pgx.Tx, owner, id string, in quests.Input, tasks *quests.Service) (FirstActionResult, error) {
	var out FirstActionResult
	ctx := r.Context()
	g, e := s.get(ctx, tx, owner, id)
	if e != nil {
		return out, e
	}
	if e := platform.MatchVersion(r, g.Version); e != nil {
		return out, e
	}
	if g.State != "active" || g.ArchivedAt != nil {
		return out, problem(409, "GOAL_CLOSED", "Выберите активную цель")
	}
	var count int
	e = tx.QueryRow(ctx, `SELECT count(*) FROM goal_quests l JOIN quests q ON q.owner_id=l.owner_id AND q.id=l.quest_id WHERE l.owner_id=$1 AND l.goal_id=$2 AND q.archived_at IS NULL AND q.status IN('todo','backlog','in_progress','postponed')`, owner, id).Scan(&count)
	if e != nil {
		return out, e
	}
	if count > 0 {
		return out, problem(409, "NEXT_ACTION_EXISTS", "У цели уже есть следующее действие. Откройте её план")
	}
	if in.ParentID != nil {
		return out, invalid("parent_id", "Первый шаг создаётся самостоятельной задачей")
	}
	if in.AreaID == nil {
		in.AreaID = g.AreaID
	}
	q, e := tasks.CreateInTx(ctx, tx, owner, in)
	if e != nil {
		return out, e
	}
	out.Task = q
	out.Goal, e = s.AttachNewQuestInTx(ctx, tx, owner, g, q)
	return out, e
}
