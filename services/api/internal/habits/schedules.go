package habits

import (
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

func periodBounds(d time.Time, frequency string, zone *time.Location) (time.Time, time.Time) {
	a := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, zone)
	switch frequency {
	case "weekly":
		a = a.AddDate(0, 0, -(int(a.Weekday())+6)%7)
		return a, a.AddDate(0, 0, 7)
	case "monthly":
		a = time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, zone)
		return a, a.AddDate(0, 1, 0)
	default:
		return a, a.AddDate(0, 0, 1)
	}
}
func scheduled(day time.Time, s Schedule) bool {
	if s.Frequency != "daily" {
		return true
	}
	d := (int(day.Weekday())+6)%7 + 1
	for _, v := range s.Weekdays {
		if d == v {
			return true
		}
	}
	return false
}
func activeSchedule(h Habit, d string) ScheduleVersion {
	v := h.Schedules[0]
	for _, next := range h.Schedules {
		if next.EffectiveFrom <= d {
			v = next
		}
	}
	return v
}

type ScheduleChange struct {
	EffectiveFrom string   `json:"effective_from"`
	Schedule      Schedule `json:"schedule"`
	Reason        string   `json:"reason"`
}

func validReason(v string) error {
	if strings.TrimSpace(v) == "" || len([]rune(v)) > 1000 || strings.ContainsRune(v, 0) {
		return invalid("reason", "Укажите причину: 1–1000 символов")
	}
	return nil
}
func (s *Service) scheduleChange(ctx context.Context, tx pgx.Tx, id string, h Habit, in ScheduleChange, apply bool) (Habit, error) {
	next, e := validateSchedule(in.Schedule)
	if e != nil {
		return h, e
	}
	if e = validReason(in.Reason); e != nil {
		return h, e
	}
	d, e := date(in.EffectiveFrom)
	if e != nil {
		return h, e
	}
	z, e := time.LoadLocation(h.Timezone)
	if e != nil {
		return h, e
	}
	a := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, z)
	if !a.After(s.clock.Now()) || in.EffectiveFrom < h.StartsOn || (h.EndsOn != nil && in.EffectiveFrom > *h.EndsOn) {
		return h, invalid("effective_from", "Выберите ещё не начавшийся период в пределах действия привычки")
	}
	old := activeSchedule(h, in.EffectiveFrom)
	oldStart, _ := periodBounds(d, old.Schedule.Frequency, z)
	newStart, _ := periodBounds(d, next.Frequency, z)
	if !a.Equal(oldStart) || !a.Equal(newStart) {
		return h, invalid("effective_from", "Изменение начинается на общей границе прежнего и нового периодов")
	}
	for _, p := range h.Pauses {
		if p.EndsOn != nil && *p.EndsOn < in.EffectiveFrom {
			continue
		}
		if p.StartsOn >= in.EffectiveFrom {
			day, _ := date(p.StartsOn)
			bound, _ := periodBounds(day, next.Frequency, z)
			if bound.Format(time.DateOnly) != p.StartsOn {
				return h, invalid("effective_from", "Новое расписание несовместимо с началом сохранённой паузы")
			}
		}
		if p.EndsOn != nil {
			day, _ := date(*p.EndsOn)
			day = day.AddDate(0, 0, 1)
			bound, _ := periodBounds(day, next.Frequency, z)
			if bound.Format(time.DateOnly) != day.Format(time.DateOnly) {
				return h, invalid("effective_from", "Новое расписание несовместимо с окончанием сохранённой паузы")
			}
		}
	}
	candidate := h.Input
	candidate.Schedule = next
	if _, e = Validate(candidate); e != nil {
		return h, e
	}
	for _, v := range h.Schedules {
		if v.EffectiveFrom >= in.EffectiveFrom {
			return h, invalid("effective_from", "Выберите дату позже уже сохранённых изменений")
		}
	}
	if !apply {
		h.Schedules = append(h.Schedules, ScheduleVersion{Version: h.Version + 1, EffectiveFrom: in.EffectiveFrom, Schedule: next, Reason: in.Reason})
		return h, nil
	}
	data, e := json.Marshal(next)
	if e != nil {
		return h, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO habit_schedules(owner_id,habit_id,version,effective_from,schedule,reason) VALUES($1,$2,$3,$4::text::date,$5,$6)`, id, h.ID, h.Version+1, in.EffectiveFrom, data, in.Reason); e != nil {
		return h, e
	}
	if _, e = tx.Exec(ctx, `UPDATE habits SET version=version+1,updated_at=$3 WHERE owner_id=$1 AND id=$2`, id, h.ID, s.clock.Now().UTC()); e != nil {
		return h, e
	}
	return s.view(ctx, tx, id, h.ID)
}

func (s *Service) mountSchedules(r chi.Router) {
	r.Post("/api/v1/habits/{id}/schedules", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in ScheduleChange) (any, error) {
			return s.mutate(r, in, func(tx pgx.Tx, id string) (any, error) {
				h, e := s.get(r.Context(), tx, id, chi.URLParam(r, "id"))
				if e != nil {
					return nil, e
				}
				if p := platform.MatchVersion(r, h.Version); p != nil {
					return nil, p
				}
				if h.ArchivedAt != nil {
					return nil, problem(409, "ARCHIVED", "Сначала восстановите привычку")
				}
				return s.scheduleChange(r.Context(), tx, id, h, in, true)
			})
		})
	})
}
