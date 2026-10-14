package habits

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type ReasonInput struct {
	Reason string `json:"reason"`
}
type ArchiveSpan struct {
	ID            int64      `json:"id"`
	ArchivedAt    time.Time  `json:"archived_at"`
	RestoredAt    *time.Time `json:"restored_at"`
	ArchiveReason string     `json:"archive_reason"`
	RestoreReason *string    `json:"restore_reason"`
}
type SchedulePreview struct {
	HabitID       string    `json:"habit_id"`
	Version       int64     `json:"version"`
	EffectiveFrom string    `json:"effective_from"`
	Current       Schedule  `json:"current"`
	Proposed      Schedule  `json:"proposed"`
	FirstStart    time.Time `json:"first_start"`
	FirstEnd      time.Time `json:"first_end"`
	Reason        string    `json:"reason"`
}
type PeriodPage struct {
	Items      []Period `json:"items"`
	NextCursor *string  `json:"next_cursor"`
}

func (s *Service) archives(ctx context.Context, tx pgx.Tx, id, key string) ([]ArchiveSpan, error) {
	rows, e := tx.Query(ctx, `SELECT id,archived_at,restored_at,archive_reason,restore_reason FROM habit_archives WHERE owner_id=$1 AND habit_id=$2 ORDER BY id`, id, key)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ArchiveSpan{}
	for rows.Next() {
		var a ArchiveSpan
		if e = rows.Scan(&a.ID, &a.ArchivedAt, &a.RestoredAt, &a.ArchiveReason, &a.RestoreReason); e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Service) archive(ctx context.Context, tx pgx.Tx, id string, h Habit, restore bool, in ReasonInput) (Habit, error) {
	if e := validReason(in.Reason); e != nil {
		return h, e
	}
	now := s.clock.Now().UTC()
	if restore {
		if h.ArchivedAt == nil {
			return h, problem(409, "NOT_ARCHIVED", "Привычка уже активна")
		}
		z, e := time.LoadLocation(h.Timezone)
		if e != nil {
			return h, e
		}
		today := now.In(z)
		_, b := periodBounds(today, activeSchedule(h, today.Format(time.DateOnly)).Schedule.Frequency, z)
		var exists bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM habit_periods WHERE owner_id=$1 AND habit_id=$2 AND starts_at<=$3 AND ends_at>$3)`, id, h.ID, now).Scan(&exists); e != nil {
			return h, e
		}
		through := b.AddDate(0, 0, -1).Format(time.DateOnly)
		if exists {
			through = today.Format(time.DateOnly)
		}
		if _, e = tx.Exec(ctx, `UPDATE habit_archives SET restored_at=$3,restore_reason=$4 WHERE owner_id=$1 AND habit_id=$2 AND restored_at IS NULL`, id, h.ID, now, in.Reason); e != nil {
			return h, e
		}
		if _, e = tx.Exec(ctx, `UPDATE habits SET archived_at=NULL,materialized_through=$3::text::date WHERE owner_id=$1 AND id=$2`, id, h.ID, through); e != nil {
			return h, e
		}
	} else {
		if h.ArchivedAt != nil {
			return h, problem(409, "ARCHIVED", "Привычка уже в архиве")
		}
		fresh, e := s.view(ctx, tx, id, h.ID)
		if e != nil {
			return h, e
		}
		if fresh.HasMore {
			return h, problem(409, "HISTORY_LOADING", "Сначала дождитесь загрузки всех периодов")
		}
		if _, e = tx.Exec(ctx, `INSERT INTO habit_archives(owner_id,habit_id,archived_at,archive_reason) VALUES($1,$2,$3,$4)`, id, h.ID, now, in.Reason); e != nil {
			return h, e
		}
		if _, e = tx.Exec(ctx, `UPDATE habits SET archived_at=$3 WHERE owner_id=$1 AND id=$2`, id, h.ID, now); e != nil {
			return h, e
		}
	}
	if e := s.bump(ctx, tx, id, h.ID); e != nil {
		return h, e
	}
	return s.view(ctx, tx, id, h.ID)
}
func (s *Service) preview(ctx context.Context, tx pgx.Tx, id string, h Habit, in ScheduleChange) (SchedulePreview, error) {
	var out SchedulePreview
	if h.ArchivedAt != nil {
		return out, problem(409, "ARCHIVED", "Сначала восстановите привычку")
	}
	_, e := s.scheduleChange(ctx, tx, id, h, in, false)
	if e != nil {
		return out, e
	}
	d, _ := date(in.EffectiveFrom)
	z, e := time.LoadLocation(h.Timezone)
	if e != nil {
		return out, e
	}
	a, b := periodBounds(d, in.Schedule.Frequency, z)
	return SchedulePreview{HabitID: h.ID, Version: h.Version, EffectiveFrom: in.EffectiveFrom, Current: activeSchedule(h, in.EffectiveFrom).Schedule, Proposed: in.Schedule, FirstStart: a.UTC(), FirstEnd: b.UTC(), Reason: in.Reason}, nil
}
func (s *Service) mountHistory(r chi.Router) {
	r.Get("/api/v1/habits/{id}/periods/{period}", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		out := Period{}
		periodID := chi.URLParam(r, "period")
		if e == nil && !uuid(periodID) {
			e = problem(404, "NOT_FOUND", "Период не найден")
		}
		if e == nil {
			e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
				if e := s.lock(r.Context(), tx, id); e != nil {
					return e
				}
				h, e := s.get(r.Context(), tx, id, chi.URLParam(r, "id"))
				if e != nil {
					return e
				}
				items, e := s.periodsFor(r.Context(), tx, id, h, &periodID)
				if e != nil {
					return e
				}
				if len(items) == 0 {
					return problem(404, "NOT_FOUND", "Период не найден")
				}
				out = items[0]
				return nil
			})
		}
		s.reply(w, r, out, e)
	})
	for _, action := range []string{"archive", "restore"} {
		restore := action == "restore"
		r.Post("/api/v1/habits/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			decode(s, w, r, func(in ReasonInput) (any, error) {
				return s.mutate(r, in, func(tx pgx.Tx, id string) (any, error) {
					h, e := s.get(r.Context(), tx, id, chi.URLParam(r, "id"))
					if e != nil {
						return nil, e
					}
					if p := platform.MatchVersion(r, h.Version); p != nil {
						return nil, p
					}
					return s.archive(r.Context(), tx, id, h, restore, in)
				})
			})
		})
	}
	r.Post("/api/v1/habits/{id}/schedules/preview", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in ScheduleChange) (any, error) {
			id, e := owner(r.Context())
			var out SchedulePreview
			if e == nil {
				e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
					if e := s.lock(r.Context(), tx, id); e != nil {
						return e
					}
					h, e := s.get(r.Context(), tx, id, chi.URLParam(r, "id"))
					if e != nil {
						return e
					}
					if p := platform.MatchVersion(r, h.Version); p != nil {
						return p
					}
					out, e = s.preview(r.Context(), tx, id, h, in)
					return e
				})
			}
			return out, e
		})
	})
	r.Get("/api/v1/habits/{id}/periods", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		out := PeriodPage{Items: []Period{}}
		before := r.URL.Query().Get("before")
		var cut time.Time
		if before != "" {
			var err error
			cut, err = time.Parse(time.RFC3339Nano, before)
			if err != nil {
				e = invalid("before", "Некорректная граница истории")
			}
		}
		if e == nil {
			e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
				if e := s.lock(r.Context(), tx, id); e != nil {
					return e
				}
				h, e := s.get(r.Context(), tx, id, chi.URLParam(r, "id"))
				if e != nil {
					return e
				}
				periods, e := s.periods(r.Context(), tx, id, h)
				if e != nil {
					return e
				}
				for i := len(periods) - 1; i >= 0; i-- {
					p := periods[i]
					if !cut.IsZero() && !p.Start.Before(cut) {
						continue
					}
					if len(out.Items) == 60 {
						cursor := out.Items[len(out.Items)-1].Start.UTC().Format(time.RFC3339Nano)
						out.NextCursor = &cursor
						break
					}
					out.Items = append(out.Items, p)
				}
				return nil
			})
		}
		s.reply(w, r, out, e)
	})
}
