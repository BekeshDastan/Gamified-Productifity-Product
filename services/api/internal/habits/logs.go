package habits

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"math"
	"net/http"
	"time"
)

type LogInput struct {
	PeriodID   string    `json:"period_id"`
	Amount     float64   `json:"amount"`
	OccurredAt time.Time `json:"occurred_at"`
}
type CorrectionInput struct {
	Amount float64 `json:"amount"`
	Reason string  `json:"reason"`
}
type Correction struct {
	ID        int64     `json:"id"`
	Before    float64   `json:"before"`
	After     float64   `json:"after"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}
type Log struct {
	ID             string       `json:"id"`
	PeriodID       string       `json:"period_id"`
	Amount         float64      `json:"amount"`
	OriginalAmount float64      `json:"original_amount"`
	OccurredAt     time.Time    `json:"occurred_at"`
	AcceptedAt     time.Time    `json:"accepted_at"`
	Backfilled     bool         `json:"backfilled"`
	Corrections    []Correction `json:"corrections"`
}

func (s *Service) bump(ctx context.Context, tx pgx.Tx, id, key string) error {
	_, e := tx.Exec(ctx, `UPDATE habits SET version=version+1,updated_at=$3 WHERE owner_id=$1 AND id=$2`, id, key, s.clock.Now().UTC())
	return e
}
func (s *Service) addLog(ctx context.Context, tx pgx.Tx, id string, h Habit, in LogInput) (Habit, error) {
	if !uuid(in.PeriodID) || !quantity(in.Amount) {
		return h, invalid("amount", "Положительная отметка до 1000000, не более трёх знаков")
	}
	if h.Kind == "binary" && in.Amount != 1 {
		return h, invalid("amount", "Одна бинарная отметка = одно выполнение")
	}
	if h.ArchivedAt != nil {
		return h, problem(409, "ARCHIVED", "Сначала восстановите привычку")
	}
	var a, b time.Time
	var excluded bool
	e := tx.QueryRow(ctx, `SELECT starts_at,ends_at,excluded FROM habit_periods WHERE owner_id=$1 AND habit_id=$2 AND id=$3`, id, h.ID, in.PeriodID).Scan(&a, &b, &excluded)
	if e == pgx.ErrNoRows {
		return h, problem(404, "NOT_FOUND", "Период не найден")
	}
	if e != nil {
		return h, e
	}
	if excluded {
		return h, invalid("period_id", "Это период отдыха или экстренного пропуска")
	}
	now := s.clock.Now().UTC()
	if in.OccurredAt.Before(a) || !in.OccurredAt.Before(b) || in.OccurredAt.After(now) {
		return h, invalid("occurred_at", "Факт внутри периода и не в будущем")
	}
	z, e := time.LoadLocation(h.Timezone)
	if e != nil {
		return h, e
	}
	backfilled := in.OccurredAt.In(z).Format(time.DateOnly) < now.In(z).Format(time.DateOnly) || !now.Before(b)
	if _, e = tx.Exec(ctx, `INSERT INTO habit_logs(owner_id,habit_id,period_id,amount,original_amount,occurred_at,accepted_at,backfilled) VALUES($1,$2,$3,$4,$4,$5,$6,$7)`, id, h.ID, in.PeriodID, in.Amount, in.OccurredAt.UTC(), now, backfilled); e != nil {
		return h, e
	}
	if e = s.bump(ctx, tx, id, h.ID); e != nil {
		return h, e
	}
	return s.view(ctx, tx, id, h.ID)
}
func (s *Service) correct(ctx context.Context, tx pgx.Tx, id string, h Habit, key string, in CorrectionInput) (Habit, error) {
	if !uuid(key) {
		return h, problem(404, "NOT_FOUND", "Отметка не найдена")
	}
	if (in.Amount != 0 && !quantity(in.Amount)) || math.IsNaN(in.Amount) || in.Amount < 0 {
		return h, invalid("amount", "Исправленное количество: 0–1000000, не более трёх знаков")
	}
	if h.Kind == "binary" && in.Amount != 0 && in.Amount != 1 {
		return h, invalid("amount", "Бинарная отметка: 0 или 1")
	}
	if e := validReason(in.Reason); e != nil {
		return h, e
	}
	var before float64
	e := tx.QueryRow(ctx, `SELECT amount FROM habit_logs WHERE owner_id=$1 AND habit_id=$2 AND id=$3 FOR UPDATE`, id, h.ID, key).Scan(&before)
	if e == pgx.ErrNoRows {
		return h, problem(404, "NOT_FOUND", "Отметка не найдена")
	}
	if e != nil {
		return h, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO habit_log_corrections(owner_id,log_id,before_amount,after_amount,reason,created_at) VALUES($1,$2,$3,$4,$5,$6)`, id, key, before, in.Amount, in.Reason, s.clock.Now().UTC()); e != nil {
		return h, e
	}
	if _, e = tx.Exec(ctx, `UPDATE habit_logs SET amount=$3 WHERE owner_id=$1 AND id=$2`, id, key, in.Amount); e != nil {
		return h, e
	}
	if e = s.bump(ctx, tx, id, h.ID); e != nil {
		return h, e
	}
	return s.view(ctx, tx, id, h.ID)
}

type LogPage struct {
	Items      []Log   `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

func (s *Service) logs(ctx context.Context, tx pgx.Tx, id string, h Habit, before string) ([]Log, error) {
	var cursor *string
	if before != "" {
		if !uuid(before) {
			return nil, invalid("before", "Некорректная граница истории")
		}
		var exists bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM habit_logs WHERE owner_id=$1 AND habit_id=$2 AND id=$3)`, id, h.ID, before).Scan(&exists); e != nil {
			return nil, e
		}
		if !exists {
			return nil, problem(404, "NOT_FOUND", "Отметка границы не найдена")
		}
		cursor = &before
	}
	rows, e := tx.Query(ctx, `SELECT id::text,period_id::text,amount,original_amount,occurred_at,accepted_at,backfilled FROM habit_logs WHERE owner_id=$1 AND habit_id=$2 AND ($3::uuid IS NULL OR (accepted_at,id)<(SELECT accepted_at,id FROM habit_logs WHERE owner_id=$1 AND habit_id=$2 AND id=$3)) ORDER BY accepted_at DESC,id DESC LIMIT 101`, id, h.ID, cursor)
	if e != nil {
		return nil, e
	}
	out := []Log{}
	for rows.Next() {
		l := Log{Corrections: []Correction{}}
		if e = rows.Scan(&l.ID, &l.PeriodID, &l.Amount, &l.OriginalAmount, &l.OccurredAt, &l.AcceptedAt, &l.Backfilled); e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, l)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for i := range out {
		rows, e := tx.Query(ctx, `SELECT id,before_amount,after_amount,reason,created_at FROM habit_log_corrections WHERE owner_id=$1 AND log_id=$2 ORDER BY id`, id, out[i].ID)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var c Correction
			if e = rows.Scan(&c.ID, &c.Before, &c.After, &c.Reason, &c.CreatedAt); e != nil {
				rows.Close()
				return nil, e
			}
			out[i].Corrections = append(out[i].Corrections, c)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (s *Service) mountLogs(r chi.Router) {
	r.Post("/api/v1/habits/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in LogInput) (any, error) {
			return s.mutate(r, in, func(tx pgx.Tx, id string) (any, error) {
				h, e := s.get(r.Context(), tx, id, chi.URLParam(r, "id"))
				if e != nil {
					return nil, e
				}
				if p := platform.MatchVersion(r, h.Version); p != nil {
					return nil, p
				}
				return s.addLog(r.Context(), tx, id, h, in)
			})
		})
	})
	r.Post("/api/v1/habits/{id}/logs/{log}/correct", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in CorrectionInput) (any, error) {
			return s.mutate(r, in, func(tx pgx.Tx, id string) (any, error) {
				h, e := s.get(r.Context(), tx, id, chi.URLParam(r, "id"))
				if e != nil {
					return nil, e
				}
				if p := platform.MatchVersion(r, h.Version); p != nil {
					return nil, p
				}
				return s.correct(r.Context(), tx, id, h, chi.URLParam(r, "log"), in)
			})
		})
	})
	r.Get("/api/v1/habits/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		out := LogPage{Items: []Log{}}
		if e == nil {
			tx, err := s.pool.Begin(r.Context())
			e = err
			if e == nil {
				defer func() { _ = tx.Rollback(r.Context()) }()
				h, err := s.get(r.Context(), tx, id, chi.URLParam(r, "id"))
				e = err
				if e == nil {
					var logs []Log
					logs, e = s.logs(r.Context(), tx, id, h, r.URL.Query().Get("before"))
					if e == nil {
						out.Items = logs
						if len(logs) > 100 {
							out.Items = logs[:100]
							cursor := out.Items[99].ID
							out.NextCursor = &cursor
						}
					}
				}
				if e == nil {
					e = tx.Commit(r.Context())
				}
			}
		}
		s.reply(w, r, out, e)
	})
}
