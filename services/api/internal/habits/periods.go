package habits

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

type Period struct {
	ID              string    `json:"id"`
	Key             string    `json:"key"`
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`
	Timezone        string    `json:"timezone"`
	ScheduleVersion int64     `json:"schedule_version"`
	Target          float64   `json:"target"`
	Amount          float64   `json:"amount"`
	Progress        int       `json:"progress"`
	State           string    `json:"state"`
	Excluded        bool      `json:"excluded"`
	EmergencyReason string    `json:"emergency_reason"`
}

func (s *Service) materialize(ctx context.Context, tx pgx.Tx, id string, h Habit) (bool, error) {
	z, e := time.LoadLocation(h.Timezone)
	if e != nil {
		return false, e
	}
	start, e := date(h.StartsOn)
	if e != nil {
		return false, e
	}
	until := s.clock.Now().In(z).Format(time.DateOnly)
	if h.EndsOn != nil && *h.EndsOn < until {
		until = *h.EndsOn
	}
	var cursor *string
	if e = tx.QueryRow(ctx, `SELECT materialized_through::text FROM habits WHERE owner_id=$1 AND id=$2`, id, h.ID).Scan(&cursor); e != nil {
		return false, e
	}
	if cursor != nil && *cursor >= until {
		return false, nil
	}
	d := start
	if cursor != nil {
		d, _ = date(*cursor)
		d = d.AddDate(0, 0, 1)
	}
	created := 0
	last := ""
	more := false
	for d.Format(time.DateOnly) <= until {
		v := activeSchedule(h, d.Format(time.DateOnly))
		a, b := periodBounds(d, v.Schedule.Frequency, z)
		key := v.Schedule.Frequency + ":" + a.Format(time.DateOnly)
		if a.Format(time.DateOnly) < h.StartsOn {
			a = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, z)
		}
		if h.EndsOn != nil && b.Format(time.DateOnly) > *h.EndsOn {
			end, _ := date(*h.EndsOn)
			b = time.Date(end.Year(), end.Month(), end.Day()+1, 0, 0, 0, 0, z)
		}
		if h.ArchivedAt != nil && !a.Before(*h.ArchivedAt) {
			break
		}
		if created == 400 {
			more = true
			break
		}
		if scheduled(d, v.Schedule) {
			tag, e := tx.Exec(ctx, `INSERT INTO habit_periods(owner_id,habit_id,period_key,starts_at,ends_at,timezone,schedule_version,target) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(owner_id,habit_id,period_key) DO NOTHING`, id, h.ID, key, a.UTC(), b.UTC(), h.Timezone, v.Version, v.Schedule.Target)
			if e != nil {
				return false, e
			}
			created += int(tag.RowsAffected())
		}
		next := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
		if !next.After(d) {
			break
		}
		last = next.AddDate(0, 0, -1).Format(time.DateOnly)
		if last > until {
			last = until
		}
		d = next
	}
	if last != "" {
		_, e = tx.Exec(ctx, `UPDATE habits SET materialized_through=$3::text::date WHERE owner_id=$1 AND id=$2`, id, h.ID, last)
	}
	return more, e
}
func (s *Service) periods(ctx context.Context, tx pgx.Tx, id string, h Habit) ([]Period, error) {
	rows, e := tx.Query(ctx, `SELECT p.id::text,p.period_key,p.starts_at,p.ends_at,p.timezone,p.schedule_version,p.target,p.excluded,p.emergency_reason,COALESCE((SELECT sum(l.amount) FROM habit_logs l WHERE l.owner_id=p.owner_id AND l.period_id=p.id),0)::double precision FROM habit_periods p WHERE p.owner_id=$1 AND p.habit_id=$2 ORDER BY p.starts_at,p.id`, id, h.ID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Period{}
	for rows.Next() {
		var p Period
		if e = rows.Scan(&p.ID, &p.Key, &p.Start, &p.End, &p.Timezone, &p.ScheduleVersion, &p.Target, &p.Excluded, &p.EmergencyReason, &p.Amount); e != nil {
			return nil, e
		}
		p.Progress = int(min(100, p.Amount/p.Target*100))
		switch {
		case p.Excluded:
			p.State = "rest"
		case p.Amount >= p.Target:
			p.State = "success"
		case s.clock.Now().Before(p.End):
			p.State = "open"
		default:
			p.State = "missed"
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Service) view(ctx context.Context, tx pgx.Tx, id, key string) (Habit, error) {
	h, e := s.get(ctx, tx, id, key)
	if e != nil {
		return h, e
	}
	h.HasMore, e = s.materialize(ctx, tx, id, h)
	if e != nil {
		return h, e
	}
	h.Periods, e = s.periods(ctx, tx, id, h)
	return h, e
}
