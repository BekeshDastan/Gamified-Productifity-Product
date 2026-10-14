package analytics

import (
	"context"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"time"
)

type FutureWeek struct {
	Range
	Observed     bool     `json:"observed"`
	FocusSeconds *int64   `json:"focus_seconds"`
	Sources      []Source `json:"sources"`
}
type FutureSelf struct {
	Timezone           string       `json:"timezone"`
	ComputedAt         time.Time    `json:"computed_at"`
	HorizonWeeks       int          `json:"horizon_weeks"`
	ObservedWeeks      int          `json:"observed_weeks"`
	Available          bool         `json:"available"`
	AverageWeeklyHours *float64     `json:"average_weekly_hours"`
	ProjectedHours     *float64     `json:"projected_hours"`
	Weeks              []FutureWeek `json:"weeks"`
	Text               string       `json:"text"`
}

func fullWeekStart(now time.Time, zone *time.Location) time.Time {
	v := now.In(zone)
	d := time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, zone)
	return d.AddDate(0, 0, -(int(d.Weekday())+6)%7)
}
func futureFromDays(now, created time.Time, timezone string, horizon int, days []Day) (FutureSelf, error) {
	out := FutureSelf{Timezone: timezone, ComputedAt: now, HorizonWeeks: horizon, Weeks: []FutureWeek{}}
	if horizon < 1 || horizon > 104 {
		return out, privateapi.Invalid("Выберите горизонт от 1 до 104 недель")
	}
	zone, e := time.LoadLocation(timezone)
	if e != nil {
		return out, e
	}
	end := fullWeekStart(now, zone)
	total := int64(0)
	for start := end.AddDate(0, 0, -28); start.Before(end); start = start.AddDate(0, 0, 7) {
		next := start.AddDate(0, 0, 7)
		w := FutureWeek{Range: Range{start.Format(time.DateOnly), next.Format(time.DateOnly)}, Observed: !start.Before(created), Sources: []Source{}}
		if w.Observed {
			seconds := int64(0)
			for _, d := range days {
				if d.Date < w.From || d.Date >= w.To {
					continue
				}
				seconds += d.FocusSeconds
				for _, s := range d.Sources {
					if s.Kind == "focus" {
						w.Sources = append(w.Sources, s)
					}
				}
			}
			w.FocusSeconds = &seconds
			total += seconds
			out.ObservedWeeks++
		}
		out.Weeks = append(out.Weeks, w)
	}
	out.Available = out.ObservedWeeks >= 2
	if !out.Available {
		out.Text = "Недостаточно данных: нужны минимум две полные наблюдавшиеся недели. Неполная текущая неделя не входит в расчёт."
		return out, nil
	}
	mean := float64(total) / float64(out.ObservedWeeks) / 3600
	projected := mean * float64(horizon)
	out.AverageWeeklyHours = &mean
	out.ProjectedHours = &projected
	out.Text = fmt.Sprintf("При сохранении записанного темпа %.2f часа подтверждённой работы в неделю за %d недель добавится около %.2f часа практики. Это арифметический сценарий, без обещания уровня знаний, работы или другого результата. Нулевые полные недели включены.", mean, horizon, projected)
	return out, nil
}
func (s *Service) future(ctx context.Context, tx pgx.Tx, owner string, horizon int) (FutureSelf, error) {
	now := s.Clock.Now()
	var timezone string
	var created time.Time
	if e := tx.QueryRow(ctx, `SELECT s.preferences->>'timezone',u.created_at FROM user_settings s JOIN users u ON u.id=s.owner_id WHERE s.owner_id=$1 FOR SHARE OF s`, owner).Scan(&timezone, &created); e != nil {
		return FutureSelf{}, e
	}
	zone, e := time.LoadLocation(timezone)
	if e != nil {
		return FutureSelf{}, e
	}
	end := fullWeekStart(now, zone)
	in := Range{end.AddDate(0, 0, -28).Format(time.DateOnly), end.Format(time.DateOnly)}
	days, e := s.collectBase(ctx, tx, owner, in, timezone)
	if e != nil {
		return FutureSelf{}, e
	}
	return futureFromDays(now, created, timezone, horizon, days)
}
func (s *Service) mountFuture(r chi.Router) {
	r.Get("/api/v1/analytics/future-self", func(w http.ResponseWriter, r *http.Request) {
		weeks, e := strconv.Atoi(r.URL.Query().Get("weeks"))
		if e != nil || weeks < 1 || weeks > 104 {
			s.Reply(w, r, nil, privateapi.Invalid("Выберите 1–104 недели"))
			return
		}
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.future(ctx, tx, owner, weeks)
		})
		s.Reply(w, r, out, e)
	})
}
