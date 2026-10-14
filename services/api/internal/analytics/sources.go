package analytics

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"sort"
	"time"
)

func (s *Service) collect(ctx context.Context, tx pgx.Tx, owner string, in Range, timezone string) ([]Day, error) {
	from, to, e := validateRange(in)
	if e != nil {
		return nil, e
	}
	zone, e := time.LoadLocation(timezone)
	if e != nil {
		return nil, e
	}
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, zone)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, zone)
	out := []Day{}
	index := map[string]int{}
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		key := d.Format(time.DateOnly)
		index[key] = len(out)
		out = append(out, Day{Date: key, Sources: []Source{}})
	}
	add := func(v Source) {
		key := v.At.In(zone).Format(time.DateOnly)
		i, ok := index[key]
		if !ok {
			return
		}
		d := &out[i]
		d.Sources = append(d.Sources, v)
		switch v.Kind {
		case "task":
			d.Completed++
		case "focus":
			d.FocusSeconds += int64(v.Value)
		case "xp":
			d.XPDelta += int64(v.Value)
		case "coins":
			d.CoinsDelta += int64(v.Value)
		}
	}
	rows, e := tx.Query(ctx, `SELECT q.id::text,q.payload->>'title',q.completed_at,q.area_id::text,q.version FROM quests q WHERE q.owner_id=$1 AND q.status='completed' AND q.completed_at>=$2 AND q.completed_at<$3 AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id) ORDER BY q.completed_at,q.id`, owner, start, end)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		v := Source{Kind: "task", Value: 1, Unit: "tasks"}
		if e = rows.Scan(&v.ID, &v.Name, &v.At, &v.AreaID, &v.Version); e != nil {
			rows.Close()
			return nil, e
		}
		v.URL = "/app/quests/" + v.ID
		add(v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	rows, e = tx.Query(ctx, `SELECT f.id::text,f.title,f.area_id::text,f.version,seg.id,seg.started_at,seg.ended_at FROM focus_sessions f JOIN focus_segments seg ON seg.owner_id=f.owner_id AND seg.session_id=f.id AND seg.kind='work' AND seg.ended_at IS NOT NULL WHERE f.owner_id=$1 AND f.state='finished' AND f.result<>'rest' AND seg.started_at<$3 AND seg.ended_at>$2 ORDER BY seg.id`, owner, start, end)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		v := Source{Kind: "focus", Unit: "seconds"}
		var segment int64
		var a, b time.Time
		if e = rows.Scan(&v.ID, &v.Name, &v.AreaID, &v.Version, &segment, &a, &b); e != nil {
			rows.Close()
			return nil, e
		}
		v.URL = "/app/focus?session_id=" + v.ID
		v.ID = fmt.Sprintf("%s/%d", v.ID, segment)
		for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
			next := day.AddDate(0, 0, 1)
			lo, hi := a, b
			if lo.Before(day) {
				lo = day
			}
			if hi.After(next) {
				hi = next
			}
			if hi.After(lo) {
				v.At = lo
				v.Value = float64(int64(hi.Sub(a)/time.Second) - int64(lo.Sub(a)/time.Second))
				if v.Value > 0 {
					add(v)
				}
			}
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	rows, e = tx.Query(ctx, `SELECT id::text,source_kind,xp_delta,coins_delta,occurred_at FROM reward_ledger WHERE owner_id=$1 AND occurred_at>=$2 AND occurred_at<$3 ORDER BY occurred_at,id`, owner, start, end)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		v := Source{URL: "/app/progress", Version: 1}
		var xp, coins int64
		if e = rows.Scan(&v.ID, &v.Name, &xp, &coins, &v.At); e != nil {
			rows.Close()
			return nil, e
		}
		if xp != 0 {
			v.Kind = "xp"
			v.Unit = "xp"
			v.Value = float64(xp)
			add(v)
		}
		if coins != 0 {
			v.Kind = "coins"
			v.Unit = "coins"
			v.Value = float64(coins)
			add(v)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for i := range out {
		sort.SliceStable(out[i].Sources, func(a, b int) bool {
			x, y := out[i].Sources[a], out[i].Sources[b]
			if !x.At.Equal(y.At) {
				return x.At.Before(y.At)
			}
			if x.Kind != y.Kind {
				return x.Kind < y.Kind
			}
			return x.ID < y.ID
		})
	}
	return out, nil
}
