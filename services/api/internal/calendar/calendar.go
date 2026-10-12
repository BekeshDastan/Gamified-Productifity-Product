package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool   *pgxpool.Pool
	clock  platform.Clock
	quests *quests.Service
}

func NewService(pool *pgxpool.Pool, clock platform.Clock, tasks *quests.Service) *Service {
	return &Service{pool, clock, tasks}
}

type Input struct {
	Title    string  `json:"title"`
	AreaID   *string `json:"area_id"`
	QuestID  *string `json:"quest_id"`
	AllDay   bool    `json:"all_day"`
	Start    string  `json:"start"`
	End      string  `json:"end"`
	Timezone string  `json:"timezone"`
}
type Event struct {
	Input
	ID           string `json:"id"`
	Version      int64  `json:"version"`
	QuestVersion int64  `json:"quest_version"`
	Source       string `json:"source"`
	Status       string `json:"status"`
	Cancelled    bool   `json:"cancelled"`
}
type Page struct {
	Items    []Event `json:"items"`
	Timezone string  `json:"timezone"`
}
type Edit struct {
	Input
	Reason       string `json:"reason"`
	QuestVersion int64  `json:"quest_version"`
	Cancelled    bool   `json:"cancelled"`
}

func problem(status int, code, message string) *platform.APIError {
	return &platform.APIError{Status: status, Code: code, Message: message}
}
func invalid(field, message string) *platform.APIError {
	p := problem(422, "VALIDATION_ERROR", "Проверьте поля календаря")
	p.Fields = map[string]string{field: message}
	return p
}
func uuid(v string) bool { var id pgtype.UUID; return id.Scan(v) == nil && id.Valid }
func owner(ctx context.Context) (string, error) {
	id, ok := platform.CurrentIdentity(ctx)
	if !ok {
		return "", problem(401, "UNAUTHORIZED", "Войдите в аккаунт")
	}
	return id.UserID, nil
}
func day(v string) (time.Time, error) {
	d, e := time.Parse("2006-01-02", v)
	if e != nil || d.Year() < 1 || d.Format("2006-01-02") != v {
		return d, invalid("start", "Укажите календарную дату")
	}
	return d, nil
}
func Validate(in Input) (Input, error) {
	in.Title = strings.TrimSpace(in.Title)
	if !utf8.ValidString(in.Title) || utf8.RuneCountInString(in.Title) < 1 || utf8.RuneCountInString(in.Title) > 200 || strings.IndexFunc(in.Title, unicode.IsControl) >= 0 {
		return in, invalid("title", "Название: 1–200 символов")
	}
	if (in.AreaID != nil && !uuid(*in.AreaID)) || (in.QuestID != nil && !uuid(*in.QuestID)) {
		return in, invalid("quest_id", "Выберите свою задачу и направление")
	}
	if in.Timezone == "" || in.Timezone == "Local" {
		return in, invalid("timezone", "Выберите IANA-часовой пояс")
	}
	if _, e := time.LoadLocation(in.Timezone); e != nil {
		return in, invalid("timezone", "Неизвестный часовой пояс")
	}
	var start, end time.Time
	var e error
	if in.AllDay {
		start, e = day(in.Start)
		if e == nil {
			end, e = day(in.End)
		}
	} else {
		start, e = time.Parse(time.RFC3339, in.Start)
		if e == nil {
			end, e = time.Parse(time.RFC3339, in.End)
		}
		if e == nil {
			in.Start = start.UTC().Format(time.RFC3339)
			in.End = end.UTC().Format(time.RFC3339)
		}
	}
	if e != nil || !end.After(start) || end.Sub(start) > 366*24*time.Hour || start.Year() < 1 || end.Year() > 9999 {
		return in, invalid("end", "Конец позже начала, продолжительность до 366 дней; конец даты не включён")
	}
	return in, nil
}
func (s *Service) lock(ctx context.Context, tx pgx.Tx, id string) error {
	var v string
	return tx.QueryRow(ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, id).Scan(&v)
}

const columns = `e.payload,e.id::text,e.version,e.cancelled,COALESCE(q.version,0),COALESCE(q.status,'planned')`

func scan(row pgx.Row) (Event, error) {
	var v Event
	var data []byte
	e := row.Scan(&data, &v.ID, &v.Version, &v.Cancelled, &v.QuestVersion, &v.Status)
	if e == nil {
		e = json.Unmarshal(data, &v.Input)
	}
	v.Source = "event"
	if v.QuestID != nil {
		v.Source = "block"
	}
	return v, e
}
func (s *Service) get(ctx context.Context, tx pgx.Tx, id, key string) (Event, error) {
	if !uuid(key) {
		return Event{}, problem(404, "NOT_FOUND", "Событие не найдено")
	}
	v, e := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM calendar_events e LEFT JOIN quests q ON q.owner_id=e.owner_id AND q.id=e.quest_id WHERE e.owner_id=$1 AND e.id=$2`, id, key))
	if errors.Is(e, pgx.ErrNoRows) {
		e = problem(404, "NOT_FOUND", "Событие не найдено")
	}
	return v, e
}
func (s *Service) labels(ctx context.Context, tx pgx.Tx, id string, in Input) error {
	if in.AreaID != nil {
		var ok bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM life_areas WHERE owner_id=$1 AND id=$2)`, id, in.AreaID).Scan(&ok); e != nil {
			return e
		}
		if !ok {
			return invalid("area_id", "Направление не найдено")
		}
	}
	if in.QuestID != nil {
		var ok bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quests WHERE owner_id=$1 AND id=$2 AND archived_at IS NULL)`, id, in.QuestID).Scan(&ok); e != nil {
			return e
		}
		if !ok {
			return problem(404, "NOT_FOUND", "Задача не найдена")
		}
	}
	return nil
}
func dates(in Input) (any, any, any, any) {
	if in.AllDay {
		return in.Start, in.End, nil, nil
	}
	return nil, nil, in.Start, in.End
}
func (s *Service) history(ctx context.Context, tx pgx.Tx, id string, before *Event, after Event, reason string) error {
	old, e := json.Marshal(before)
	if e != nil {
		return e
	}
	fresh, e := json.Marshal(after)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO calendar_event_history(owner_id,event_id,before_state,after_state,reason,created_at) VALUES($1,$2,$3,$4,$5,$6)`, id, after.ID, old, fresh, reason, s.clock.Now().UTC())
	return e
}
func (s *Service) insert(ctx context.Context, tx pgx.Tx, id string, in Input) (Event, error) {
	if e := s.labels(ctx, tx, id, in); e != nil {
		return Event{}, e
	}
	data, e := json.Marshal(in)
	if e != nil {
		return Event{}, e
	}
	a, b, c, d := dates(in)
	var key string
	e = tx.QueryRow(ctx, `INSERT INTO calendar_events(owner_id,quest_id,payload,all_day,start_date,end_date,starts_at,ends_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5::text::date,$6::text::date,$7::text::timestamptz,$8::text::timestamptz,$9,$9) RETURNING id::text`, id, in.QuestID, data, in.AllDay, a, b, c, d, s.clock.Now().UTC()).Scan(&key)
	if e != nil {
		return Event{}, e
	}
	v, e := s.get(ctx, tx, id, key)
	if e == nil {
		e = s.history(ctx, tx, id, nil, v, "Создание")
	}
	return v, e
}
func (s *Service) idempotent(r *http.Request, in any, fn func(pgx.Tx) (any, error)) (any, error) {
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
	store := storage.IdempotencyStore{DB: s.pool, Now: s.clock.Now}
	out, e := store.Execute(r.Context(), storage.Scope{UserID: id, Method: r.Method, RouteKey: r.URL.Path, Key: r.Header.Get("Idempotency-Key")}, data, func(tx pgx.Tx) (storage.Result, error) {
		if e := s.lock(r.Context(), tx, id); e != nil {
			return storage.Result{}, e
		}
		v, e := fn(tx)
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
		e = problem(409, "IDEMPOTENCY_MISMATCH", "Ключ уже использован для другого запроса")
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
			p = problem(500, "INTERNAL_ERROR", "Не удалось сохранить календарь")
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
		r.Get("/api/v1/calendar", func(w http.ResponseWriter, r *http.Request) { v, e := s.List(r); s.reply(w, r, v, e) })
		r.Post("/api/v1/calendar/events", func(w http.ResponseWriter, r *http.Request) {
			decode(s, w, r, func(in Input) (any, error) {
				valid, e := Validate(in)
				if e != nil {
					return nil, e
				}
				id, e := owner(r.Context())
				if e != nil {
					return nil, e
				}
				return s.idempotent(r, valid, func(tx pgx.Tx) (any, error) { return s.insert(r.Context(), tx, id, valid) })
			})
		})
	})
}
