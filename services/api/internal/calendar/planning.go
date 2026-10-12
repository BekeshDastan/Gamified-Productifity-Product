package calendar

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

// Planning calls these methods inside its transaction while holding the owner lock.
func (s *Service) CreateInTx(ctx context.Context, tx pgx.Tx, id string, in Input) (Event, error) {
	valid, e := Validate(in)
	if e != nil {
		return Event{}, e
	}
	return s.insert(ctx, tx, id, valid)
}
func (s *Service) TimedInTx(ctx context.Context, tx pgx.Tx, id string, from, to time.Time) ([]Event, error) {
	rows, e := tx.Query(ctx, `SELECT `+columns+` FROM calendar_events e LEFT JOIN quests q ON q.owner_id=e.owner_id AND q.id=e.quest_id WHERE e.owner_id=$1 AND NOT e.all_day AND NOT e.cancelled AND (q.id IS NULL OR q.archived_at IS NULL) AND e.starts_at<$3 AND e.ends_at>$2 ORDER BY e.starts_at,e.id LIMIT 2001`, id, from, to)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) > 2000 {
		return nil, problem(422, "RANGE_LIMIT", "Слишком много блоков в дне")
	}
	return out, nil
}
func (s *Service) MaterializeInTx(ctx context.Context, tx pgx.Tx, id string, from, to time.Time) (bool, error) {
	return s.materialize(ctx, tx, id, from, to)
}
