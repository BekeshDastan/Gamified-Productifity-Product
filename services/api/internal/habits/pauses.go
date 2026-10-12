package habits

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type PauseInput struct {
	StartsOn string  `json:"starts_on"`
	EndsOn   *string `json:"ends_on"`
	Reason   string  `json:"reason"`
}
type Pause struct {
	PauseInput
	ID        string    `json:"id"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}
type ResumeInput struct {
	ResumesOn string `json:"resumes_on"`
	Reason    string `json:"reason"`
}
type EmergencyInput struct {
	PeriodID string `json:"period_id"`
	Reason   string `json:"reason"`
}

func (s *Service) pauses(ctx context.Context, tx pgx.Tx, id, key string) ([]Pause, error) {
	rows, e := tx.Query(ctx, `SELECT id::text,starts_on::text,ends_on::text,reason,version,created_at FROM habit_pauses WHERE owner_id=$1 AND habit_id=$2 ORDER BY starts_on,id`, id, key)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Pause{}
	for rows.Next() {
		var p Pause
		if e = rows.Scan(&p.ID, &p.StartsOn, &p.EndsOn, &p.Reason, &p.Version, &p.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Service) futureBoundary(h Habit, value string) error {
	d, e := date(value)
	if e != nil {
		return e
	}
	z, e := time.LoadLocation(h.Timezone)
	if e != nil {
		return e
	}
	a := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, z)
	if !a.After(s.clock.Now()) || value < h.StartsOn || (h.EndsOn != nil && value > *h.EndsOn) {
		return invalid("date", "Выберите ещё не начавшийся период в пределах действия привычки")
	}
	v := activeSchedule(h, value)
	start, _ := periodBounds(d, v.Schedule.Frequency, z)
	if !a.Equal(start) {
		return invalid("date", "Пауза и возобновление начинаются на границе периода")
	}
	return nil
}
func pauseReason(h Habit, a, b time.Time) string {
	for _, p := range h.Pauses {
		if a.Format(time.DateOnly) >= p.StartsOn && (p.EndsOn == nil || b.AddDate(0, 0, -1).Format(time.DateOnly) <= *p.EndsOn) {
			return p.Reason
		}
	}
	return ""
}
func (s *Service) addPause(ctx context.Context, tx pgx.Tx, id string, h Habit, in PauseInput) (Habit, error) {
	if h.ArchivedAt != nil {
		return h, problem(409, "ARCHIVED", "Сначала восстановите привычку")
	}
	if e := validReason(in.Reason); e != nil {
		return h, e
	}
	if e := s.futureBoundary(h, in.StartsOn); e != nil {
		return h, e
	}
	if len(h.Pauses) >= 100 {
		return h, invalid("starts_on", "До 100 пауз")
	}
	if in.EndsOn != nil {
		d, e := date(*in.EndsOn)
		if e != nil || *in.EndsOn < in.StartsOn {
			return h, invalid("ends_on", "Конец паузы не раньше начала")
		}
		if e = s.futureBoundary(h, d.AddDate(0, 0, 1).Format(time.DateOnly)); e != nil {
			return h, e
		}
	}
	for _, p := range h.Pauses {
		if (p.EndsOn == nil || in.StartsOn <= *p.EndsOn) && (in.EndsOn == nil || p.StartsOn <= *in.EndsOn) {
			return h, invalid("starts_on", "Паузы не пересекаются")
		}
	}
	if _, e := tx.Exec(ctx, `INSERT INTO habit_pauses(owner_id,habit_id,starts_on,ends_on,reason,created_at) VALUES($1,$2,$3::text::date,$4::text::date,$5,$6)`, id, h.ID, in.StartsOn, in.EndsOn, in.Reason, s.clock.Now().UTC()); e != nil {
		return h, e
	}
	if e := s.bump(ctx, tx, id, h.ID); e != nil {
		return h, e
	}
	return s.view(ctx, tx, id, h.ID)
}
func (s *Service) resume(ctx context.Context, tx pgx.Tx, id string, h Habit, key string, in ResumeInput) (Habit, error) {
	if !uuid(key) {
		return h, problem(404, "NOT_FOUND", "Пауза не найдена")
	}
	if e := validReason(in.Reason); e != nil {
		return h, e
	}
	if e := s.futureBoundary(h, in.ResumesOn); e != nil {
		return h, e
	}
	var old *Pause
	for _, p := range h.Pauses {
		if p.ID == key {
			v := p
			old = &v
		}
	}
	if old == nil {
		return h, problem(404, "NOT_FOUND", "Пауза не найдена")
	}
	if in.ResumesOn <= old.StartsOn || (old.EndsOn != nil && in.ResumesOn > *old.EndsOn) {
		return h, invalid("resumes_on", "Возобновление внутри ещё не завершённой паузы")
	}
	d, _ := date(in.ResumesOn)
	end := d.AddDate(0, 0, -1).Format(time.DateOnly)
	if _, e := tx.Exec(ctx, `UPDATE habit_pauses SET ends_on=$4::text::date,reason=reason || E'\nВозобновление: ' || $5,version=version+1 WHERE owner_id=$1 AND habit_id=$2 AND id=$3`, id, h.ID, key, end, in.Reason); e != nil {
		return h, e
	}
	if e := s.bump(ctx, tx, id, h.ID); e != nil {
		return h, e
	}
	return s.view(ctx, tx, id, h.ID)
}
func (s *Service) emergency(ctx context.Context, tx pgx.Tx, id string, h Habit, in EmergencyInput) (Habit, error) {
	if e := validReason(in.Reason); e != nil {
		return h, e
	}
	if !uuid(in.PeriodID) {
		return h, problem(404, "NOT_FOUND", "Период не найден")
	}
	h, e := s.view(ctx, tx, id, h.ID)
	if e != nil {
		return h, e
	}
	var period *Period
	for _, p := range h.Periods {
		if p.ID == in.PeriodID {
			v := p
			period = &v
		}
	}
	if period == nil {
		return h, problem(404, "NOT_FOUND", "Период не найден")
	}
	if period.Excluded || !s.clock.Now().Before(period.End) || s.clock.Now().Before(period.Start) || period.Amount >= period.Target {
		return h, invalid("period_id", "Экстренный пропуск доступен только для текущей незавершённой нормы")
	}
	if _, e = tx.Exec(ctx, `UPDATE habit_periods SET excluded=true,emergency_reason=$3 WHERE owner_id=$1 AND id=$2`, id, in.PeriodID, in.Reason); e != nil {
		return h, e
	}
	if e = s.bump(ctx, tx, id, h.ID); e != nil {
		return h, e
	}
	return s.view(ctx, tx, id, h.ID)
}
func (s *Service) mountPauses(r chi.Router) {
	r.Post("/api/v1/habits/{id}/pauses", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in PauseInput) (any, error) {
			return s.mutate(r, in, func(tx pgx.Tx, id string) (any, error) {
				h, e := s.get(r.Context(), tx, id, chi.URLParam(r, "id"))
				if e != nil {
					return nil, e
				}
				if p := platform.MatchVersion(r, h.Version); p != nil {
					return nil, p
				}
				return s.addPause(r.Context(), tx, id, h, in)
			})
		})
	})
	r.Post("/api/v1/habits/{id}/pauses/{pause}/resume", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in ResumeInput) (any, error) {
			return s.mutate(r, in, func(tx pgx.Tx, id string) (any, error) {
				h, e := s.get(r.Context(), tx, id, chi.URLParam(r, "id"))
				if e != nil {
					return nil, e
				}
				if p := platform.MatchVersion(r, h.Version); p != nil {
					return nil, p
				}
				return s.resume(r.Context(), tx, id, h, chi.URLParam(r, "pause"), in)
			})
		})
	})
	r.Post("/api/v1/habits/{id}/emergency", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in EmergencyInput) (any, error) {
			return s.mutate(r, in, func(tx pgx.Tx, id string) (any, error) {
				h, e := s.get(r.Context(), tx, id, chi.URLParam(r, "id"))
				if e != nil {
					return nil, e
				}
				if p := platform.MatchVersion(r, h.Version); p != nil {
					return nil, p
				}
				return s.emergency(r.Context(), tx, id, h, in)
			})
		})
	})
}
