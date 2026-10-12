package planning

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type Move struct {
	QuestID      string `json:"quest_id"`
	QuestVersion int64  `json:"quest_version"`
	Reason       string `json:"reason"`
}
type Block struct {
	Move
	Start string `json:"start"`
	End   string `json:"end"`
}

func bounds(date, zone string) (time.Time, time.Time, error) {
	d, e := day(date)
	if e != nil {
		return d, d, e
	}
	z, e := time.LoadLocation(zone)
	if e != nil {
		return d, d, e
	}
	a := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, z)
	b := time.Date(d.Year(), d.Month(), d.Day()+1, 0, 0, 0, 0, z)
	return a, b, nil
}
func wall(date, zone, value string, end bool) (time.Time, error) {
	d, e := day(date)
	if e != nil {
		return d, e
	}
	m, e := minute(value, end)
	if e != nil {
		return d, e
	}
	z, e := time.LoadLocation(zone)
	if e != nil {
		return d, e
	}
	if m == 1440 {
		d = d.AddDate(0, 0, 1)
		m = 0
	}
	v, ok := calendar.ResolveWall(d, m/60, m%60, z)
	if !ok {
		return v, invalid("start", "Это местное время отсутствует из-за перевода часов")
	}
	return v, nil
}
func (s *Service) selected(r *http.Request, tx pgx.Tx, id, date string, in Move) (Plan, error) {
	p, e := s.get(r.Context(), tx, id, date)
	if e != nil {
		return p, e
	}
	if !uuid(in.QuestID) {
		return p, invalid("quest_id", "Выберите задачу плана")
	}
	if e = reason(in.Reason); e != nil {
		return p, e
	}
	found := false
	for _, key := range p.QuestIDs {
		if key == in.QuestID {
			found = true
		}
	}
	if p.ID == "" || !found {
		return p, invalid("quest_id", "Сначала сохраните задачу в плане")
	}
	if err := platform.MatchVersion(r, p.Version); err != nil {
		return p, err
	}
	return p, nil
}
func (s *Service) externalChange(ctx context.Context, tx pgx.Tx, id string, p Plan, why string) error {
	next := p
	next.Version++
	if _, e := tx.Exec(ctx, `UPDATE day_plans SET version=version+1,updated_at=$3 WHERE owner_id=$1 AND id=$2`, id, p.ID, s.clock.Now().UTC()); e != nil {
		return e
	}
	if p.ConfirmedAt != nil {
		return s.journal(ctx, tx, id, p, next, "addendum", why)
	}
	return nil
}
func (s *Service) mountBlocks(r chi.Router) {
	r.Post("/api/v1/plans/day/{date}/move", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in Move) (any, error) {
			date := chi.URLParam(r, "date")
			if _, e := day(date); e != nil {
				return nil, e
			}
			return s.mutate(r, in, func(tx pgx.Tx, id string) (any, error) {
				p, e := s.selected(r, tx, id, date, in)
				if e != nil {
					return nil, e
				}
				q, e := s.quests.ScheduleInTx(r.Context(), tx, id, in.QuestID, in.QuestVersion, date, in.Reason)
				if e != nil {
					return nil, e
				}
				if e = s.externalChange(r.Context(), tx, id, p, "Перенос задачи: "+q.Title+". "+in.Reason); e != nil {
					return nil, e
				}
				return s.get(r.Context(), tx, id, date)
			})
		})
	})
	r.Post("/api/v1/plans/day/{date}/blocks", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in Block) (any, error) {
			date := chi.URLParam(r, "date")
			if _, e := day(date); e != nil {
				return nil, e
			}
			return s.mutate(r, in, func(tx pgx.Tx, id string) (any, error) {
				p, e := s.selected(r, tx, id, date, in.Move)
				if e != nil {
					return nil, e
				}
				a, e := wall(date, p.Timezone, in.Start, false)
				if e != nil {
					return nil, e
				}
				b, e := wall(date, p.Timezone, in.End, true)
				if e != nil {
					return nil, e
				}
				if !b.After(a) {
					return nil, invalid("end", "Конец блока должен быть позже начала")
				}
				q, e := s.quests.ScheduleInTx(r.Context(), tx, id, in.QuestID, in.QuestVersion, date, in.Reason)
				if e != nil {
					return nil, e
				}
				v, e := s.calendar.CreateInTx(r.Context(), tx, id, calendar.Input{Title: q.Title, AreaID: q.AreaID, QuestID: &q.ID, Start: a.UTC().Format(time.RFC3339), End: b.UTC().Format(time.RFC3339), Timezone: p.Timezone})
				if e != nil {
					return nil, e
				}
				if e = s.externalChange(r.Context(), tx, id, p, "Добавлен блок "+v.ID+". "+in.Reason); e != nil {
					return nil, e
				}
				return v, nil
			})
		})
	})
}
