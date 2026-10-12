package quests

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Transition struct {
	Reason          string     `json:"reason"`
	PlannedDate     *string    `json:"planned_date"`
	ReturnToBacklog bool       `json:"return_to_backlog"`
	ShiftDeadline   bool       `json:"shift_deadline"`
	Deadline        *time.Time `json:"deadline"`
}
type Event struct {
	Details   map[string][]string `json:"details"`
	ID        int64               `json:"id"`
	Kind      string              `json:"kind"`
	Reason    string              `json:"reason"`
	Before    *Quest              `json:"before"`
	After     Quest               `json:"after"`
	CreatedAt time.Time           `json:"created_at"`
}

func (s *Service) event(ctx context.Context, tx pgx.Tx, id, kind, reason string, before *Quest, after Quest) error {
	old, err := json.Marshal(before)
	if err != nil {
		return err
	}
	fresh, err := json.Marshal(after)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO task_events(owner_id,quest_id,kind,reason,before_state,after_state,quest_version,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, after.ID, kind, reason, old, fresh, after.Version, s.clock.Now().UTC())
	if err == nil && s.observer != nil {
		err = s.observer(ctx, tx, id, kind, before, after)
	}
	return err
}
func nextStatus(current, action string) (string, error) {
	switch action {
	case "start":
		if current == "backlog" || current == "todo" || current == "postponed" {
			return "in_progress", nil
		}
	case "ready":
		if current == "backlog" || current == "in_progress" || current == "postponed" {
			return "todo", nil
		}
	case "reopen":
		if current == "completed" || current == "failed" || current == "skipped" {
			return "todo", nil
		}
	case "complete", "skip", "fail":
		if current == "backlog" || current == "todo" || current == "in_progress" || current == "postponed" {
			return map[string]string{"complete": "completed", "skip": "skipped", "fail": "failed"}[action], nil
		}
	case "postpone":
		if current == "todo" || current == "in_progress" || current == "postponed" {
			return "postponed", nil
		}
	}
	return "", problem(409, "INVALID_TRANSITION", "Этот переход статуса недоступен")
}
func (s *Service) transitionTx(r *http.Request, tx pgx.Tx, id, key, action string, in Transition) (Quest, error) {
	ctx := r.Context()
	if err := s.lockOwner(ctx, tx, id); err != nil {
		return Quest{}, err
	}
	q, err := get(ctx, tx, id, key, true)
	if err != nil {
		return q, err
	}
	if p := platform.MatchVersion(r, q.Version); p != nil {
		return q, p
	}
	if q.ArchivedAt != nil {
		return q, problem(409, "ARCHIVED", "Сначала восстановите задачу")
	}
	next, err := nextStatus(q.Status, action)
	if err != nil {
		return q, err
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if !utf8.ValidString(in.Reason) || utf8.RuneCountInString(in.Reason) > 1000 || strings.ContainsRune(in.Reason, 0) {
		return q, invalid("reason", "Причина: до 1000 символов")
	}
	if (action == "postpone" || action == "skip" || action == "fail") && in.Reason == "" {
		return q, invalid("reason", "Укажите причину")
	}
	before := q
	if action == "start" || action == "complete" {
		if err := s.checkBlocked(ctx, tx, id, key); err != nil {
			return q, err
		}
	}
	if action == "complete" {
		sum, e := summary(ctx, tx, id, key)
		if e != nil {
			return q, e
		}
		if sum.Container && sum.RequiredLeaves > sum.CompletedLeaves {
			return q, problem(409, "CHILDREN_INCOMPLETE", "Сначала завершите обязательные подзадачи")
		}
	}
	if action == "postpone" {
		if in.ReturnToBacklog {
			if in.PlannedDate != nil {
				return q, invalid("planned_date", "Выберите дату или возврат в идеи")
			}
			next = "backlog"
			q.PlannedDate = nil
		} else {
			if in.PlannedDate == nil {
				return q, invalid("planned_date", "Укажите новую дату")
			}
			q.PlannedDate = in.PlannedDate
		}
		if in.ShiftDeadline {
			q.Deadline = in.Deadline
		} else if in.Deadline != nil {
			return q, invalid("deadline", "Подтвердите отдельное изменение дедлайна")
		}
	} else if in.PlannedDate != nil || in.ReturnToBacklog || in.ShiftDeadline || in.Deadline != nil {
		return q, invalid("planned_date", "Изменение даты доступно через перенос")
	}
	q.Status = next
	if _, err = Validate(q.Input); err != nil {
		return q, err
	}
	if next == "completed" {
		now := s.clock.Now().UTC()
		q.CompletedAt = &now
	} else {
		q.CompletedAt = nil
	}
	data, err := json.Marshal(q.Input)
	if err != nil {
		return q, err
	}
	q, err = scan(tx.QueryRow(ctx, `UPDATE quests SET payload=$3,status=$4,completed_at=$5,planned_date=$7::text::date,deadline=$8,version=version+1,updated_at=$6 WHERE owner_id=$1 AND id=$2 RETURNING `+columns, id, key, data, next, q.CompletedAt, s.clock.Now().UTC(), q.PlannedDate, q.Deadline))
	if err == nil {
		err = s.event(ctx, tx, id, action, in.Reason, &before, q)
	}
	if err == nil && q.ParentID != nil {
		err = s.rollup(ctx, tx, id, *q.ParentID)
	}
	if err == nil {
		err = s.rollupDependents(ctx, tx, id, q.ID)
	}
	return q, err
}
func (s *Service) idempotent(r *http.Request, input any, fn func(pgx.Tx) (any, error)) (any, error) {
	id, e := owner(r.Context())
	if e != nil {
		return nil, e
	}
	data, e := json.Marshal(input)
	if e != nil {
		return nil, e
	}
	store := storage.IdempotencyStore{DB: s.pool, Now: s.clock.Now}
	result, e := store.Execute(r.Context(), storage.Scope{UserID: id, Method: r.Method, RouteKey: r.URL.Path, Key: r.Header.Get("Idempotency-Key")}, data, func(tx pgx.Tx) (storage.Result, error) {
		value, e := fn(tx)
		if e != nil {
			return storage.Result{}, e
		}
		body, e := json.Marshal(value)
		return storage.Result{Status: 200, Body: body}, e
	})
	switch {
	case errors.Is(e, storage.ErrInvalidScope):
		return nil, invalid("idempotency_key", "Укажите Idempotency-Key: 16–128 букв, цифр, . _ -")
	case errors.Is(e, storage.ErrIdempotencyMismatch):
		return nil, problem(409, "IDEMPOTENCY_MISMATCH", "Ключ уже использован с другими данными")
	case errors.Is(e, storage.ErrOperationInProgress):
		return nil, problem(409, "OPERATION_IN_PROGRESS", "Операция выполняется")
	}
	if e != nil {
		return nil, e
	}
	return json.RawMessage(result.Body), nil
}
func (s *Service) mountLifecycle(r chi.Router) {
	for _, action := range []string{"start", "ready", "complete", "reopen", "postpone", "skip", "fail"} {
		r.Post("/api/v1/quests/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			decode(w, r, func(in Transition) (any, error) {
				id, e := owner(r.Context())
				if e != nil {
					return nil, e
				}
				return s.idempotent(r, in, func(tx pgx.Tx) (any, error) { return s.transitionTx(r, tx, id, chi.URLParam(r, "id"), action, in) })
			}, s)
		})
	}
	r.Get("/api/v1/quests/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		key := chi.URLParam(r, "id")
		if _, e = get(r.Context(), s.pool, id, key, false); e != nil {
			s.reply(w, r, nil, e)
			return
		}
		// Bounded pages: clients request older history using a monotonically increasing event ID.
		before := int64(1<<63 - 1)
		if raw := r.URL.Query().Get("before"); raw != "" {
			before, e = strconv.ParseInt(raw, 10, 64)
			if e != nil || before < 1 {
				s.reply(w, r, nil, invalid("before", "Укажите номер события"))
				return
			}
		}
		rows, e := s.pool.Query(r.Context(), `SELECT id,kind,reason,before_state,after_state,created_at,details FROM task_events WHERE owner_id=$1 AND quest_id=$2 AND id<$3 ORDER BY id DESC LIMIT 50`, id, key, before)
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		defer rows.Close()
		out := []Event{}
		for rows.Next() {
			var v Event
			var old, fresh, details []byte
			e = rows.Scan(&v.ID, &v.Kind, &v.Reason, &old, &fresh, &v.CreatedAt, &details)
			if e == nil {
				e = json.Unmarshal(details, &v.Details)
			}
			if e == nil {
				e = json.Unmarshal(old, &v.Before)
			}
			if e == nil {
				e = json.Unmarshal(fresh, &v.After)
			}
			if e != nil {
				break
			}
			out = append(out, v)
		}
		if e == nil {
			e = rows.Err()
		}
		s.reply(w, r, out, e)
	})
}
