package goals

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type MetricInput struct {
	Value        float64 `json:"value"`
	Unit         string  `json:"unit"`
	OccurredOn   string  `json:"occurred_on"`
	Note         string  `json:"note"`
	Reason       string  `json:"reason"`
	EntryVersion int64   `json:"entry_version"`
}
type Entry struct {
	ID         int64     `json:"id"`
	Value      float64   `json:"value"`
	Unit       string    `json:"unit"`
	OccurredOn string    `json:"occurred_on"`
	Note       string    `json:"note"`
	Version    int64     `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
}
type Revision struct {
	Entry
	Reason string `json:"reason"`
}

func (s *Service) setupMetric(ctx context.Context, tx pgx.Tx, id, key string, in Input) error {
	var count int
	if e := tx.QueryRow(ctx, `SELECT count(*) FROM goal_metric_entries WHERE owner_id=$1 AND goal_id=$2`, id, key).Scan(&count); e != nil {
		return e
	}
	if count > 0 {
		var unit string
		if e := tx.QueryRow(ctx, `SELECT unit FROM goal_metrics WHERE owner_id=$1 AND goal_id=$2`, id, key).Scan(&unit); e != nil {
			return e
		}
		if in.ProgressType != "metric" || unit != in.Unit {
			return invalid("unit", "У записи уже есть результаты: единицу и тип можно менять только в новой цели")
		}
	}
	if in.ProgressType == "metric" {
		_, e := tx.Exec(ctx, `INSERT INTO goal_metrics(owner_id,goal_id,start_value,target,unit) VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner_id,goal_id) DO UPDATE SET start_value=excluded.start_value,target=excluded.target,unit=excluded.unit`, id, key, in.Start, in.Target, in.Unit)
		return e
	}
	_, e := tx.Exec(ctx, `DELETE FROM goal_metrics WHERE owner_id=$1 AND goal_id=$2`, id, key)
	return e
}
func (s *Service) metricProgress(ctx context.Context, tx pgx.Tx, id string, g Goal) (float64, float64, error) {
	var current, progress float64
	e := tx.QueryRow(ctx, `SELECT current::float8,GREATEST(0,LEAST(100,100*(current-$3::numeric)/($4::numeric-$3::numeric)))::float8 FROM
 (SELECT $3::numeric+COALESCE(sum(value),0) current FROM goal_metric_entries WHERE owner_id=$1 AND goal_id=$2) totals`, id, g.ID, g.Start, g.Target).Scan(&current, &progress)
	if e != nil {
		return 0, 0, e
	}
	return progress, current, nil
}
func (s *Service) idempotent(r *http.Request, in any, fn func(pgx.Tx) (any, error)) (any, error) {
	id, e := owner(r.Context())
	if e != nil {
		return nil, e
	}
	data, e := json.Marshal(struct {
		Input   any    `json:"input"`
		Version string `json:"version"`
	}{in, r.Header.Get("If-Match")})
	if e != nil {
		return nil, e
	}
	store := storage.IdempotencyStore{DB: s.pool, Now: s.clock.Now}
	out, e := store.Execute(r.Context(), storage.Scope{UserID: id, Method: r.Method, RouteKey: r.URL.Path, Key: r.Header.Get("Idempotency-Key")}, data, func(tx pgx.Tx) (storage.Result, error) {
		v, e := fn(tx)
		if e != nil {
			return storage.Result{}, e
		}
		data, e := json.Marshal(v)
		return storage.Result{Status: 200, Body: data}, e
	})
	switch {
	case errors.Is(e, storage.ErrInvalidScope):
		return nil, invalid("idempotency_key", "Укажите ключ повторного запроса")
	case errors.Is(e, storage.ErrIdempotencyMismatch):
		return nil, problem(409, "IDEMPOTENCY_MISMATCH", "Ключ уже использован")
	case errors.Is(e, storage.ErrOperationInProgress):
		return nil, problem(409, "OPERATION_IN_PROGRESS", "Запрос выполняется")
	}
	if e != nil {
		return nil, e
	}
	return json.RawMessage(out.Body), nil
}
func before(r *http.Request) (int64, error) {
	raw := r.URL.Query().Get("before")
	if raw == "" {
		return math.MaxInt64, nil
	}
	n, e := strconv.ParseInt(raw, 10, 64)
	if e != nil || n < 1 {
		return 0, invalid("before", "Некорректная страница")
	}
	return n, nil
}
func (s *Service) mountMetrics(r chi.Router) {
	for _, method := range []string{"POST", "PATCH"} {
		path := "/api/v1/goals/{id}/entries"
		if method == "PATCH" {
			path += "/{entry}"
		}
		r.MethodFunc(method, path, func(w http.ResponseWriter, r *http.Request) {
			decode(s, w, r, func(in MetricInput) (any, error) {
				id, e := owner(r.Context())
				if e != nil {
					return nil, e
				}
				if !amount(in.Value) || !date(&in.OccurredOn) || !text(in.Note, 1000) {
					return nil, invalid("value", "Неотрицательное значение до 1 млрд, до 3 знаков после запятой; укажите дату")
				}
				if e = reason(in.Reason); e != nil {
					return nil, e
				}
				return s.idempotent(r, in, func(tx pgx.Tx) (any, error) {
					// Corrections remain possible for a confirmed goal and can reverse its achievement.
					if e := s.lock(r.Context(), tx, id); e != nil {
						return nil, e
					}
					g, e := s.get(r.Context(), tx, id, chi.URLParam(r, "id"))
					if e != nil {
						return nil, e
					}
					if p := platform.MatchVersion(r, g.Version); p != nil {
						return nil, p
					}
					if g.ProgressType != "metric" || g.ArchivedAt != nil || (method == "POST" && !strings.Contains("|draft|active|paused|", "|"+g.State+"|")) {
						return nil, problem(409, "INVALID_TRANSITION", "Цель не принимает новые результаты")
					}
					if in.Unit != g.Unit {
						return nil, invalid("unit", "Единица должна совпадать с единицей цели")
					}
					var entryID int64
					if method == "POST" {
						if in.Value <= 0 {
							return nil, invalid("value", "Добавляемое значение должно быть больше нуля")
						}
						var count int
						if e = tx.QueryRow(r.Context(), `SELECT count(*) FROM goal_metric_entries WHERE owner_id=$1 AND goal_id=$2`, id, g.ID).Scan(&count); e != nil {
							return nil, e
						}
						if count >= 10000 {
							return nil, invalid("value", "Не более 10000 записей в цели")
						}
						e = tx.QueryRow(r.Context(), `INSERT INTO goal_metric_entries(owner_id,goal_id,metric_id,value,unit,occurred_on,note,created_at) SELECT $1,$2,id,$3,$4,$5::text::date,$6,$7 FROM goal_metrics WHERE owner_id=$1 AND goal_id=$2 RETURNING id`, id, g.ID, in.Value, in.Unit, in.OccurredOn, in.Note, s.clock.Now().UTC()).Scan(&entryID)
					} else {
						entryID, e = strconv.ParseInt(chi.URLParam(r, "entry"), 10, 64)
						if e != nil || entryID < 1 {
							return nil, problem(404, "NOT_FOUND", "Запись не найдена")
						}
						var version int64
						e = tx.QueryRow(r.Context(), `SELECT version FROM goal_metric_entries WHERE owner_id=$1 AND goal_id=$2 AND id=$3`, id, g.ID, entryID).Scan(&version)
						if errors.Is(e, pgx.ErrNoRows) {
							return nil, problem(404, "NOT_FOUND", "Запись не найдена")
						}
						if e != nil {
							return nil, e
						}
						if version != in.EntryVersion {
							return nil, problem(409, "VERSION_CONFLICT", "Результат уже изменён")
						}
						if strings.TrimSpace(in.Reason) == "" {
							return nil, invalid("reason", "Укажите причину исправления")
						}
						_, e = tx.Exec(r.Context(), `UPDATE goal_metric_entries SET value=$4,occurred_on=$5::text::date,note=$6,version=version+1 WHERE owner_id=$1 AND goal_id=$2 AND id=$3`, id, g.ID, entryID, in.Value, in.OccurredOn, in.Note)
					}
					if e != nil {
						return nil, e
					}
					_, e = tx.Exec(r.Context(), `INSERT INTO goal_metric_revisions(owner_id,entry_id,version,value,unit,occurred_on,note,reason,created_at) SELECT owner_id,id,version,value,unit,occurred_on,note,$3,$4 FROM goal_metric_entries WHERE owner_id=$1 AND id=$2`, id, entryID, in.Reason, s.clock.Now().UTC())
					if e != nil {
						return nil, e
					}
					kind := "metric_added"
					if method == "PATCH" {
						kind = "metric_corrected"
					}
					return s.touch(r.Context(), tx, id, g, kind, in.Reason)
				})
			})
		})
	}
	for _, history := range []bool{false, true} {
		path := "/api/v1/goals/{id}/entries"
		if history {
			path += "/{entry}/history"
		}
		r.Get(path, func(w http.ResponseWriter, r *http.Request) {
			id, e := owner(r.Context())
			if e != nil {
				s.reply(w, r, nil, e)
				return
			}
			g, e := s.get(r.Context(), s.pool, id, chi.URLParam(r, "id"))
			if e != nil {
				s.reply(w, r, nil, e)
				return
			}
			cursor, e := before(r)
			if e != nil {
				s.reply(w, r, nil, e)
				return
			}
			sql := `SELECT id,value::float8,unit,occurred_on::text,note,version,created_at,'' FROM goal_metric_entries WHERE owner_id=$1 AND goal_id=$2 AND id<$3 ORDER BY id DESC LIMIT 50`
			args := []any{id, g.ID, cursor}
			if history {
				key, e := strconv.ParseInt(chi.URLParam(r, "entry"), 10, 64)
				if e != nil || key < 1 {
					s.reply(w, r, nil, problem(404, "NOT_FOUND", "Запись не найдена"))
					return
				}
				var exists bool
				e = s.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM goal_metric_entries WHERE owner_id=$1 AND goal_id=$2 AND id=$3)`, id, g.ID, key).Scan(&exists)
				if e != nil || !exists {
					s.reply(w, r, nil, problem(404, "NOT_FOUND", "Запись не найдена"))
					return
				}
				sql = `SELECT id,value::float8,unit,occurred_on::text,note,version,created_at,reason FROM goal_metric_revisions WHERE owner_id=$1 AND entry_id=$2 AND id<$3 ORDER BY id DESC LIMIT 50`
				args = []any{id, key, cursor}
			}
			rows, e := s.pool.Query(r.Context(), sql, args...)
			if e != nil {
				s.reply(w, r, nil, e)
				return
			}
			defer rows.Close()
			out := []Revision{}
			for rows.Next() {
				var v Revision
				e = rows.Scan(&v.ID, &v.Value, &v.Unit, &v.OccurredOn, &v.Note, &v.Version, &v.CreatedAt, &v.Reason)
				if e != nil {
					s.reply(w, r, nil, e)
					return
				}
				out = append(out, v)
			}
			s.reply(w, r, out, rows.Err())
		})
	}
}
