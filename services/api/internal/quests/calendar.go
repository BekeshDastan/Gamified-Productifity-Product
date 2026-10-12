package quests

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

// ScheduleInTx changes the same task used by lists and goal projections.
// Calendar service holds the owner lock before updating events and tasks.
func (s *Service) ScheduleInTx(ctx context.Context, tx pgx.Tx, id, key string, version int64, planned, reason string) (Quest, error) {
	q, e := get(ctx, tx, id, key, true)
	if e != nil {
		return q, e
	}
	if version != q.Version {
		p := problem(409, "VERSION_CONFLICT", "Задача изменилась. Загрузите свежую версию")
		p.CurrentVersion = &q.Version
		return q, p
	}
	if q.ArchivedAt != nil || q.Status == "completed" || q.Status == "failed" || q.Status == "skipped" {
		return q, problem(409, "HISTORY_LOCKED", "Завершённая история не переносится. Сначала откройте задачу заново")
	}
	before := q
	q.PlannedDate = &planned
	if before.PlannedDate != nil && *before.PlannedDate != planned {
		q.Status = "postponed"
	} else if q.Status == "backlog" {
		q.Status = "todo"
	}
	in, e := Validate(q.Input)
	if e != nil {
		return q, e
	}
	q.Input = in
	if before.PlannedDate != nil && *before.PlannedDate == planned {
		return q, nil
	}
	data, e := json.Marshal(in)
	if e != nil {
		return q, e
	}
	q, e = scan(tx.QueryRow(ctx, `UPDATE quests SET payload=$3,planned_date=$4::text::date,status=$5,version=version+1,updated_at=$6 WHERE owner_id=$1 AND id=$2 RETURNING `+columns, id, key, data, planned, q.Status, s.clock.Now().UTC()))
	if e == nil {
		e = s.event(ctx, tx, id, "postponed", reason, &before, q)
	}
	return q, e
}
