package habits

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

// EvidenceInTx requires the caller to hold the owner profile/progression locks.
// It advances bounded materialization before evaluating past missed obligations.
type Evidence struct {
	ID      string
	Title   string
	AreaID  *string
	Periods []Period
	Stats   Stats
}

func (s *Service) EvidenceInTx(ctx context.Context, tx pgx.Tx, owner string, advance ...bool) ([]Evidence, bool, error) {
	rows, e := tx.Query(ctx, "SELECT id::text FROM habits WHERE owner_id=$1 ORDER BY created_at,id", owner)
	if e != nil {
		return nil, false, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, false, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, false, e
	}
	out := []Evidence{}
	for _, id := range ids {
		h, e := s.get(ctx, tx, owner, id)
		if e != nil {
			return nil, false, e
		}
		if len(advance) == 0 || advance[0] {
			more, e := s.materialize(ctx, tx, owner, h)
			if e != nil {
				return nil, false, e
			}
			if more {
				return out, false, nil
			}
		} else {
			zone, e := time.LoadLocation(h.Timezone)
			if e != nil {
				return nil, false, e
			}
			until := s.clock.Now().In(zone).Format(time.DateOnly)
			if h.EndsOn != nil && *h.EndsOn < until {
				until = *h.EndsOn
			}
			if until >= h.StartsOn {
				var cursor *string
				if e = tx.QueryRow(ctx, "SELECT materialized_through::text FROM habits WHERE owner_id=$1 AND id=$2", owner, h.ID).Scan(&cursor); e != nil {
					return nil, false, e
				}
				if cursor == nil || *cursor < until {
					return out, false, nil
				}
			}
		}
		p, e := s.periods(ctx, tx, owner, h)
		if e != nil {
			return nil, false, e
		}
		out = append(out, Evidence{h.ID, h.Title, h.AreaID, p, CalculateStats(p, s.clock.Now())})
	}
	return out, true, nil
}
