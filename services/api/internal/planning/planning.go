package planning

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Window struct {
	Start string `json:"start"`
	End   string `json:"end"`
}
type Input struct {
	AvailableMinutes int      `json:"available_minutes"`
	Windows          []Window `json:"windows"`
	QuestIDs         []string `json:"quest_ids"`
	TopIDs           []string `json:"top_ids"`
	Rest             bool     `json:"rest"`
}
type Item struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Version          int64      `json:"version"`
	EstimatedMinutes int        `json:"estimated_minutes"`
	Weight           int        `json:"weight"`
	Status           string     `json:"status"`
	Archived         bool       `json:"archived"`
	CompletedAt      *time.Time `json:"completed_at"`
}
type Snapshot struct {
	Input  Input            `json:"input"`
	Items  []Item           `json:"items"`
	Blocks []calendar.Event `json:"blocks"`
}
type Plan struct {
	Input
	ID           string        `json:"id"`
	Date         string        `json:"date"`
	Timezone     string        `json:"timezone"`
	Version      int64         `json:"version"`
	ConfirmedAt  *time.Time    `json:"confirmed_at"`
	Snapshot     *Snapshot     `json:"snapshot"`
	Items        []Item        `json:"items"`
	Changes      []Change      `json:"changes"`
	Progress     Progress      `json:"progress"`
	Availability *Availability `json:"availability"`
}
type Service struct {
	pool     *pgxpool.Pool
	clock    platform.Clock
	quests   *quests.Service
	calendar *calendar.Service
}

func NewService(pool *pgxpool.Pool, clock platform.Clock, q *quests.Service, c *calendar.Service) *Service {
	return &Service{pool, clock, q, c}
}
func problem(status int, code, message string) *platform.APIError {
	return &platform.APIError{Status: status, Code: code, Message: message}
}
func invalid(field, message string) *platform.APIError {
	p := problem(422, "VALIDATION_ERROR", "Проверьте поля плана")
	p.Fields = map[string]string{field: message}
	return p
}
func day(v string) (time.Time, error) {
	d, e := time.Parse("2006-01-02", v)
	if e != nil || d.Format("2006-01-02") != v || d.Year() < 1 || d.Year() > 9998 {
		return d, invalid("date", "Укажите календарную дату")
	}
	return d, nil
}
func uuid(v string) bool { var id pgtype.UUID; return id.Scan(v) == nil && id.Valid }
func owner(ctx context.Context) (string, error) {
	v, ok := platform.CurrentIdentity(ctx)
	if !ok {
		return "", problem(401, "UNAUTHORIZED", "Войдите в аккаунт")
	}
	return v.UserID, nil
}
func emptyInput() Input { return Input{Windows: []Window{}, QuestIDs: []string{}, TopIDs: []string{}} }
func (s *Service) lock(ctx context.Context, tx pgx.Tx, id string) error {
	var v string
	return tx.QueryRow(ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, id).Scan(&v)
}
func minute(v string, end bool) (int, error) {
	if end && v == "24:00" {
		return 1440, nil
	}
	t, e := time.Parse("15:04", v)
	if e != nil || t.Format("15:04") != v {
		return 0, invalid("windows", "Время окна: ЧЧ:ММ")
	}
	return t.Hour()*60 + t.Minute(), nil
}
func Validate(in Input) (Input, error) {
	if in.AvailableMinutes < 0 || in.AvailableMinutes > 1440 || len(in.Windows) > 8 || len(in.QuestIDs) > 100 || len(in.TopIDs) > 3 {
		return in, invalid("available_minutes", "До 1440 минут, 8 окон, 100 задач и 3 главных задач")
	}
	if in.Windows == nil {
		in.Windows = []Window{}
	}
	if in.QuestIDs == nil {
		in.QuestIDs = []string{}
	}
	if in.TopIDs == nil {
		in.TopIDs = []string{}
	}
	for _, w := range in.Windows {
		a, e := minute(w.Start, false)
		if e != nil {
			return in, e
		}
		b, e := minute(w.End, true)
		if e != nil {
			return in, e
		}
		if a >= b {
			return in, invalid("windows", "Конец окна должен быть позже начала в том же дне")
		}
	}
	ids := map[string]bool{}
	for _, id := range in.QuestIDs {
		if !uuid(id) || ids[id] {
			return in, invalid("quest_ids", "Выберите разные задачи")
		}
		ids[id] = true
	}
	top := map[string]bool{}
	for _, id := range in.TopIDs {
		if !ids[id] || top[id] {
			return in, invalid("top_ids", "Главные задачи должны входить в план и не повторяться")
		}
		top[id] = true
	}
	if in.Rest && (len(in.QuestIDs) > 0 || in.AvailableMinutes != 0 || len(in.Windows) > 0) {
		return in, invalid("rest", "Для дня отдыха снимите задачи и рабочие окна, задайте 0 минут")
	}
	return in, nil
}
func (s *Service) items(ctx context.Context, tx pgx.Tx, id string, keys []string, validate bool) ([]Item, error) {
	out := []Item{}
	for _, key := range keys {
		var v Item
		var leaf bool
		e := tx.QueryRow(ctx, `SELECT q.id::text,q.payload->>'title',q.version,COALESCE((q.payload->>'estimated_minutes')::int,0),CASE q.payload->>'priority' WHEN 'high' THEN 3 WHEN 'low' THEN 1 ELSE 2 END,q.status,q.archived_at IS NOT NULL,q.completed_at,NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id AND c.archived_at IS NULL) FROM quests q WHERE q.owner_id=$1 AND q.id=$2`, id, key).Scan(&v.ID, &v.Title, &v.Version, &v.EstimatedMinutes, &v.Weight, &v.Status, &v.Archived, &v.CompletedAt, &leaf)
		if errors.Is(e, pgx.ErrNoRows) {
			return out, problem(404, "NOT_FOUND", "Задача не найдена")
		}
		if e != nil {
			return out, e
		}
		if validate && (v.Archived || !leaf) {
			return out, invalid("quest_ids", "Выберите активные листовые задачи, без контейнеров")
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) get(ctx context.Context, tx pgx.Tx, id, date string) (Plan, error) {
	p := Plan{Input: emptyInput(), Date: date, Items: []Item{}, Changes: []Change{}}
	var data, snapshot []byte
	e := tx.QueryRow(ctx, `SELECT id::text,timezone,payload,version,confirmed_at,snapshot FROM day_plans WHERE owner_id=$1 AND local_date=$2::text::date`, id, date).Scan(&p.ID, &p.Timezone, &data, &p.Version, &p.ConfirmedAt, &snapshot)
	if errors.Is(e, pgx.ErrNoRows) {
		e = tx.QueryRow(ctx, `SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1`, id).Scan(&p.Timezone)
		return p, e
	}
	if e != nil {
		return p, e
	}
	if e = json.Unmarshal(data, &p.Input); e != nil {
		return p, e
	}
	if snapshot != nil {
		if e = json.Unmarshal(snapshot, &p.Snapshot); e != nil {
			return p, e
		}
	}
	p.Items, e = s.items(ctx, tx, id, p.QuestIDs, false)
	if e == nil {
		e = s.project(ctx, tx, id, &p)
	}
	return p, e
}
func (s *Service) save(ctx context.Context, tx pgx.Tx, id, date string, in Input, version int64, amendment ...bool) (Plan, error) {
	items, e := s.items(ctx, tx, id, in.QuestIDs, true)
	if e != nil {
		return Plan{}, e
	}
	old, e := s.get(ctx, tx, id, date)
	if e != nil {
		return old, e
	}
	if old.Version != version {
		return old, problem(409, "VERSION_CONFLICT", "План изменился. Загрузите свежую версию")
	}
	if _, e := windowsFor(date, old.Timezone, in.Windows); e != nil {
		return old, e
	}
	if old.ConfirmedAt != nil && (len(amendment) == 0 || !amendment[0]) {
		return old, problem(409, "PLAN_CONFIRMED", "Подтверждённый план меняется дополнением с причиной")
	}
	data, e := json.Marshal(in)
	if e != nil {
		return old, e
	}
	e = tx.QueryRow(ctx, `INSERT INTO day_plans(owner_id,local_date,timezone,payload,created_at,updated_at) VALUES($1,$2::text::date,$3,$4,$5,$5) ON CONFLICT(owner_id,local_date) DO UPDATE SET payload=excluded.payload,version=day_plans.version+1,updated_at=excluded.updated_at RETURNING id::text`, id, date, old.Timezone, data, s.clock.Now().UTC()).Scan(&old.ID)
	if e != nil {
		return old, e
	}
	if _, e = tx.Exec(ctx, `DELETE FROM day_plan_items WHERE owner_id=$1 AND plan_id=$2`, id, old.ID); e != nil {
		return old, e
	}
	for i, q := range items {
		if _, e = tx.Exec(ctx, `INSERT INTO day_plan_items(owner_id,plan_id,quest_id,position) VALUES($1,$2,$3,$4)`, id, old.ID, q.ID, i); e != nil {
			return old, e
		}
	}
	return s.get(ctx, tx, id, date)
}
func (s *Service) mutate(r *http.Request, in any, fn func(pgx.Tx, string) (any, error)) (any, error) {
	id, e := owner(r.Context())
	if e != nil {
		return nil, e
	}
	data, e := json.Marshal(struct {
		Input   any
		Version string
	}{in, r.Header.Get("If-Match")})
	if e != nil {
		return nil, e
	}
	out, e := (storage.IdempotencyStore{DB: s.pool, Now: s.clock.Now}).Execute(r.Context(), storage.Scope{UserID: id, Method: r.Method, RouteKey: r.URL.Path, Key: r.Header.Get("Idempotency-Key")}, data, func(tx pgx.Tx) (storage.Result, error) {
		if e := s.lock(r.Context(), tx, id); e != nil {
			return storage.Result{}, e
		}
		v, e := fn(tx, id)
		if e != nil {
			return storage.Result{}, e
		}
		b, e := json.Marshal(v)
		return storage.Result{Status: 200, Body: b}, e
	})
	switch {
	case errors.Is(e, storage.ErrInvalidScope):
		e = invalid("idempotency_key", "Нужен ключ запроса: 16–128 символов")
	case errors.Is(e, storage.ErrIdempotencyMismatch):
		e = problem(409, "IDEMPOTENCY_MISMATCH", "Ключ использован для другого запроса")
	case errors.Is(e, storage.ErrOperationInProgress):
		e = problem(409, "OPERATION_IN_PROGRESS", "Запрос выполняется")
	}
	if e != nil {
		return nil, e
	}
	return json.RawMessage(out.Body), nil
}
func (s *Service) reply(w http.ResponseWriter, r *http.Request, v any, e error) {
	if e != nil {
		var p *platform.APIError
		if !errors.As(e, &p) {
			p = problem(500, "INTERNAL_ERROR", "Не удалось обработать план")
		}
		platform.WriteError(w, r, p)
		return
	}
	platform.WriteData(w, r, s.clock, 200, v)
}
func decode[T any](s *Service, w http.ResponseWriter, r *http.Request, fn func(T) (any, error)) {
	in, p := platform.DecodeJSON[T](r)
	if p != nil {
		platform.WriteError(w, r, p)
		return
	}
	v, e := fn(in)
	s.reply(w, r, v, e)
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	router.Group(func(r chi.Router) {
		r.Use(protect)
		s.mountCommitments(r)
		s.mountBlocks(r)
		r.Get("/api/v1/plans/day/{date}", func(w http.ResponseWriter, r *http.Request) {
			date := chi.URLParam(r, "date")
			_, e := day(date)
			id, oe := owner(r.Context())
			if e == nil {
				e = oe
			}
			var p Plan
			if e == nil {
				e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error { var err error; p, err = s.view(r.Context(), tx, id, date); return err })
			}
			s.reply(w, r, p, e)
		})
		r.Put("/api/v1/plans/day/{date}", func(w http.ResponseWriter, r *http.Request) {
			decode(s, w, r, func(in Input) (any, error) {
				date := chi.URLParam(r, "date")
				if _, e := day(date); e != nil {
					return nil, e
				}
				valid, e := Validate(in)
				if e != nil {
					return nil, e
				}
				return s.mutate(r, valid, func(tx pgx.Tx, id string) (any, error) {
					old, e := s.get(r.Context(), tx, id, date)
					if e != nil {
						return nil, e
					}
					if p := matchVersion(r, old.Version); p != nil {
						return nil, p
					}
					return s.save(r.Context(), tx, id, date, valid, old.Version)
				})
			})
		})
	})
}

func matchVersion(r *http.Request, current int64) *platform.APIError {
	if r.Header.Get("If-Match") == `"0"` && current > 0 {
		return problem(409, "VERSION_CONFLICT", "План уже создан. Загрузите свежую версию")
	}
	if current == 0 {
		if r.Header.Get("If-Match") == `"0"` {
			return nil
		}
		return problem(409, "VERSION_CONFLICT", "План ещё не создан. Обновите данные")
	}
	return platform.MatchVersion(r, current)
}

// Canonical reasons are shared by confirmations, amendments and template application.
func reason(v string) error {
	if strings.TrimSpace(v) == "" || len([]rune(v)) > 1000 || strings.ContainsRune(v, 0) {
		return invalid("reason", "Укажите причину: 1–1000 символов")
	}
	return nil
}
