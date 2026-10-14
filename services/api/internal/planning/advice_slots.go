package planning

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

// FreeWorkInTx requires explicit work windows and available minutes. It never
// assumes an unconfigured day is 24 hours of work. All-day commitments block it.
func (s *Service) FreeWorkInTx(ctx context.Context, tx pgx.Tx, owner, date string, from, to time.Time) ([]Interval, error) {
	p, e := s.get(ctx, tx, owner, date)
	if e != nil {
		return nil, e
	}
	out := []Interval{}
	if p.Rest || len(p.Windows) == 0 || p.AvailableMinutes == 0 {
		return out, nil
	}
	windows, e := windowsFor(date, p.Timezone, p.Windows)
	if e != nil {
		return nil, e
	}
	blocks, e := s.calendar.DayCommitmentsInTx(ctx, tx, owner, date, p.Timezone)
	if e != nil {
		return nil, e
	}
	busy := []Interval{}
	for _, b := range blocks {
		if b.AllDay {
			return out, nil
		}
		a, e := time.Parse(time.RFC3339, b.Start)
		if e != nil {
			return nil, e
		}
		z, e := time.Parse(time.RFC3339, b.End)
		if e != nil {
			return nil, e
		}
		busy = append(busy, Interval{a, z})
	}
	remaining := p.AvailableMinutes
	for _, w := range subtract(windows, busy) {
		if remaining <= 0 {
			break
		}
		v, ok := intersection(w, Interval{from, to})
		if !ok {
			continue
		}
		duration := min(remaining, int(v.End.Sub(v.Start)/time.Minute))
		if duration <= 0 {
			continue
		}
		v.End = v.Start.Add(time.Duration(duration) * time.Minute)
		out = append(out, v)
		remaining -= duration
	}
	return out, nil
}
