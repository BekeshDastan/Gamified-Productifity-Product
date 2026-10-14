package habits

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// ReminderPeriodInTx reads canonical corrected amounts and pause/archive spans.
func (s *Service) ReminderPeriodInTx(ctx context.Context, tx pgx.Tx, owner, habitID, periodID string) (Period, int64, bool, error) {
	var exists bool
	if !uuid(habitID) || !uuid(periodID) {
		return Period{}, 0, false, nil
	}
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM habits h JOIN habit_periods p ON p.owner_id=h.owner_id AND p.habit_id=h.id WHERE h.owner_id=$1 AND h.id=$2 AND p.id=$3)`, owner, habitID, periodID).Scan(&exists)
	if e != nil || !exists {
		return Period{}, 0, false, e
	}
	h, e := s.get(ctx, tx, owner, habitID)
	if e != nil {
		return Period{}, 0, false, e
	}
	periods, e := s.periods(ctx, tx, owner, h)
	if e != nil {
		return Period{}, 0, false, e
	}
	for _, p := range periods {
		if p.ID == periodID {
			return p, h.Version, h.ArchivedAt == nil && !p.Excluded && p.Amount < p.Target && s.clock.Now().Before(p.End), nil
		}
	}
	return Period{}, 0, false, nil
}
