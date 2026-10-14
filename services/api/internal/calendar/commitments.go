package calendar

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

// DayCommitmentsInTx includes date-only obligations as well as timed blocks.
// The caller holds the owner lock; recurrence materialization preserves confirmed series.
func (s *Service) DayCommitmentsInTx(ctx context.Context, tx pgx.Tx, owner, date, timezone string) ([]Event, error) {
	d, e := day(date)
	if e != nil {
		return nil, e
	}
	zone, e := time.LoadLocation(timezone)
	if e != nil {
		return nil, e
	}
	pending, e := s.MaterializeInTx(ctx, tx, owner, d.AddDate(0, 0, -1), d.AddDate(0, 0, 2))
	if e != nil {
		return nil, e
	}
	if pending {
		return nil, problem(409, "MATERIALIZING", "Повторяющиеся события ещё обновляются. Обновите предпросмотр")
	}
	start := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, zone)
	end := start.AddDate(0, 0, 1)
	rows, e := tx.Query(ctx, `SELECT `+columns+` FROM calendar_events e LEFT JOIN quests q ON q.owner_id=e.owner_id AND q.id=e.quest_id WHERE e.owner_id=$1 AND NOT e.cancelled AND (q.id IS NULL OR q.archived_at IS NULL) AND ((e.all_day AND e.start_date<=$2::text::date AND e.end_date>$2::text::date) OR (NOT e.all_day AND e.starts_at<$4 AND e.ends_at>$3)) ORDER BY e.id LIMIT 2001`, owner, date, start, end)
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
		return nil, problem(422, "RANGE_LIMIT", "Слишком много обязательств в дне")
	}
	return out, nil
}
