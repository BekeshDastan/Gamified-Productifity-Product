package planning

import (
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type Change struct {
	Kind   string    `json:"kind"`
	Before Plan      `json:"before"`
	After  Plan      `json:"after"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}
type Progress struct {
	Committed       int  `json:"committed"`
	Completed       int  `json:"completed"`
	TotalWeight     int  `json:"total_weight"`
	CompletedWeight int  `json:"completed_weight"`
	Score           *int `json:"score"`
}

func (s *Service) journal(ctx context.Context, tx pgx.Tx, id string, before, after Plan, kind, why string) error {
	// Journal snapshots do not recursively contain their own journal or live projections.
	before.Changes = nil
	after.Changes = nil
	before.Progress = Progress{}
	after.Progress = Progress{}
	a, e := json.Marshal(before)
	if e != nil {
		return e
	}
	b, e := json.Marshal(after)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO plan_changes(owner_id,plan_id,kind,before_state,after_state,reason,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, after.ID, kind, a, b, why, s.clock.Now().UTC())
	return e
}
func (s *Service) project(ctx context.Context, tx pgx.Tx, id string, p *Plan) error {
	p.Changes = []Change{}
	if p.ID == "" {
		return nil
	}
	rows, e := tx.Query(ctx, `SELECT kind,before_state,after_state,reason,created_at FROM plan_changes WHERE owner_id=$1 AND plan_id=$2 ORDER BY id`, id, p.ID)
	if e != nil {
		return e
	}
	for rows.Next() {
		var v Change
		var a, b []byte
		if e = rows.Scan(&v.Kind, &a, &b, &v.Reason, &v.At); e != nil {
			rows.Close()
			return e
		}
		if e = json.Unmarshal(a, &v.Before); e != nil {
			rows.Close()
			return e
		}
		if e = json.Unmarshal(b, &v.After); e != nil {
			rows.Close()
			return e
		}
		p.Changes = append(p.Changes, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if p.Snapshot == nil || p.Snapshot.Input.Rest {
		return nil
	}
	keys := []string{}
	for _, v := range p.Snapshot.Items {
		keys = append(keys, v.ID)
	}
	live, e := s.items(ctx, tx, id, keys, false)
	if e != nil {
		return e
	}
	zone, e := time.LoadLocation(p.Timezone)
	if e != nil {
		return e
	}
	for i, v := range p.Snapshot.Items {
		p.Progress.Committed++
		p.Progress.TotalWeight += v.Weight
		q := live[i]
		if q.Status == "completed" && q.CompletedAt != nil && q.CompletedAt.In(zone).Format("2006-01-02") == p.Date {
			p.Progress.Completed++
			p.Progress.CompletedWeight += v.Weight
		}
	}
	if p.Progress.TotalWeight > 0 {
		score := (100*p.Progress.CompletedWeight + p.Progress.TotalWeight/2) / p.Progress.TotalWeight
		p.Progress.Score = &score
	}
	return nil
}
func (s *Service) confirm(r *http.Request, tx pgx.Tx, id, date string) (Plan, error) {
	p, e := s.get(r.Context(), tx, id, date)
	if e != nil {
		return p, e
	}
	if p.ID == "" {
		return p, problem(409, "PLAN_MISSING", "Сначала сохраните черновик")
	}
	if err := platform.MatchVersion(r, p.Version); err != nil {
		return p, err
	}
	if p.ConfirmedAt != nil {
		return p, problem(409, "PLAN_CONFIRMED", "Исходный план уже подтверждён")
	}
	items, e := s.items(r.Context(), tx, id, p.QuestIDs, true)
	if e != nil {
		return p, e
	}
	before := p
	from, to, e := bounds(p.Date, p.Timezone)
	if e != nil {
		return p, e
	}
	d, _ := day(p.Date)
	more, e := s.calendar.MaterializeInTx(r.Context(), tx, id, d, d.AddDate(0, 0, 1))
	if e != nil {
		return p, e
	}
	if more {
		return p, problem(409, "MATERIALIZING", "Догрузите календарный день перед подтверждением")
	}
	blocks, e := s.calendar.TimedInTx(r.Context(), tx, id, from, to)
	if e != nil {
		return p, e
	}
	p.Snapshot = &Snapshot{Input: p.Input, Items: items, Blocks: blocks}
	now := s.clock.Now().UTC()
	p.ConfirmedAt = &now
	p.Version++
	data, e := json.Marshal(p.Snapshot)
	if e != nil {
		return p, e
	}
	if _, e = tx.Exec(r.Context(), `UPDATE day_plans SET snapshot=$3,confirmed_at=$4,version=version+1,updated_at=$4 WHERE owner_id=$1 AND id=$2`, id, p.ID, data, now); e != nil {
		return p, e
	}
	if e = s.journal(r.Context(), tx, id, before, p, "confirmed", "Подтверждён исходный план"); e != nil {
		return p, e
	}
	return s.get(r.Context(), tx, id, date)
}

type Amendment struct {
	Input  Input  `json:"input"`
	Reason string `json:"reason"`
}

func (s *Service) mountCommitments(r chi.Router) {
	r.Post("/api/v1/plans/day/{date}/confirm", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in struct{}) (any, error) {
			date := chi.URLParam(r, "date")
			if _, e := day(date); e != nil {
				return nil, e
			}
			return s.mutate(r, in, func(tx pgx.Tx, id string) (any, error) { return s.confirm(r, tx, id, date) })
		})
	})
	r.Post("/api/v1/plans/day/{date}/addendum", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in Amendment) (any, error) {
			date := chi.URLParam(r, "date")
			if _, e := day(date); e != nil {
				return nil, e
			}
			valid, e := Validate(in.Input)
			if e != nil {
				return nil, e
			}
			in.Input = valid
			if e = reason(in.Reason); e != nil {
				return nil, e
			}
			return s.mutate(r, in, func(tx pgx.Tx, id string) (any, error) {
				old, e := s.get(r.Context(), tx, id, date)
				if e != nil {
					return nil, e
				}
				if p := platform.MatchVersion(r, old.Version); p != nil {
					return nil, p
				}
				if old.ConfirmedAt == nil {
					return nil, problem(409, "PLAN_DRAFT", "Сначала подтвердите исходный план")
				}
				next, e := s.save(r.Context(), tx, id, date, in.Input, old.Version, true)
				if e != nil {
					return nil, e
				}
				if e = s.journal(r.Context(), tx, id, old, next, "addendum", in.Reason); e != nil {
					return nil, e
				}
				return s.get(r.Context(), tx, id, date)
			})
		})
	})
}
