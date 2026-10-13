package habits

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type Schedule struct {
	Frequency string  `json:"frequency"`
	Weekdays  []int   `json:"weekdays"`
	Target    float64 `json:"target"`
}
type Input struct {
	StarterID *string  `json:"starter_id,omitempty"`
	Title     string   `json:"title"`
	Kind      string   `json:"kind"`
	Unit      string   `json:"unit"`
	Minimum   float64  `json:"minimum"`
	AreaID    *string  `json:"area_id"`
	StartsOn  string   `json:"starts_on"`
	EndsOn    *string  `json:"ends_on"`
	Schedule  Schedule `json:"schedule"`
}
type ScheduleVersion struct {
	Version       int64    `json:"version"`
	EffectiveFrom string   `json:"effective_from"`
	Schedule      Schedule `json:"schedule"`
	Reason        string   `json:"reason"`
}
type Habit struct {
	Reward   *progression.Reward `json:"reward,omitempty"`
	Archives []ArchiveSpan       `json:"archives"`
	AsOf     time.Time           `json:"as_of"`
	Pauses   []Pause             `json:"pauses"`
	Stats    Stats               `json:"stats"`
	Periods  []Period            `json:"periods"`
	HasMore  bool                `json:"has_more"`
	Input
	ID         string            `json:"id"`
	Version    int64             `json:"version"`
	Timezone   string            `json:"timezone"`
	ArchivedAt *time.Time        `json:"archived_at"`
	Schedules  []ScheduleVersion `json:"schedules"`
}
type Service struct {
	progression *progression.Service
	pool        *pgxpool.Pool
	clock       platform.Clock
}

func NewService(pool *pgxpool.Pool, clock platform.Clock) *Service {
	return &Service{pool: pool, clock: clock}
}
func problem(status int, code, message string) *platform.APIError {
	return &platform.APIError{Status: status, Code: code, Message: message}
}
func invalid(field, message string) *platform.APIError {
	p := problem(422, "VALIDATION_ERROR", "Проверьте поля привычки")
	p.Fields = map[string]string{field: message}
	return p
}
func owner(ctx context.Context) (string, error) {
	v, ok := platform.CurrentIdentity(ctx)
	if !ok {
		return "", problem(401, "UNAUTHORIZED", "Войдите в аккаунт")
	}
	return v.UserID, nil
}
func date(v string) (time.Time, error) {
	d, e := time.Parse(time.DateOnly, v)
	if e != nil || d.Format(time.DateOnly) != v || d.Year() < 1 || d.Year() > 9998 {
		return d, invalid("date", "Укажите календарную дату")
	}
	return d, nil
}
func uuid(v string) bool { var id pgtype.UUID; return id.Scan(v) == nil && id.Valid }
func quantity(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0 && v <= 1000000 && math.Abs(v*1000-math.Round(v*1000)) < 0.000001
}
func validateSchedule(s Schedule) (Schedule, error) {
	if s.Frequency != "daily" && s.Frequency != "weekly" && s.Frequency != "monthly" {
		return s, invalid("schedule.frequency", "Выберите дни, неделю или месяц")
	}
	if !quantity(s.Target) {
		return s, invalid("schedule.target", "Норма: положительное число до 1000000, не более трёх знаков")
	}
	if s.Weekdays == nil {
		s.Weekdays = []int{}
	}
	seen := map[int]bool{}
	for _, d := range s.Weekdays {
		if d < 1 || d > 7 || seen[d] {
			return s, invalid("schedule.weekdays", "Дни 1–7 без повторов")
		}
		seen[d] = true
	}
	if s.Frequency == "daily" && len(s.Weekdays) == 0 {
		return s, invalid("schedule.weekdays", "Выберите запланированные дни")
	}
	if s.Frequency != "daily" && len(s.Weekdays) > 0 {
		return s, invalid("schedule.weekdays", "Недельная и месячная нормы относятся ко всему периоду")
	}
	return s, nil
}
func Validate(in Input) (Input, error) {
	if in.StarterID != nil && !uuid(*in.StarterID) {
		return in, invalid("starter_id", "Стартовая привычка не найдена")
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Unit = strings.TrimSpace(in.Unit)
	if !utf8.ValidString(in.Title) || len([]rune(in.Title)) < 1 || len([]rune(in.Title)) > 200 || strings.ContainsRune(in.Title, 0) {
		return in, invalid("title", "Название: 1–200 символов")
	}
	if in.Kind != "binary" && in.Kind != "count" && in.Kind != "duration" {
		return in, invalid("kind", "Выберите тип привычки")
	}
	if in.Unit == "" || len([]rune(in.Unit)) > 40 || !utf8.ValidString(in.Unit) || strings.ContainsRune(in.Unit, 0) {
		return in, invalid("unit", "Укажите единицу, до 40 символов")
	}
	if !quantity(in.Minimum) {
		return in, invalid("minimum", "Минимальный шаг должен быть положительным")
	}
	var e error
	in.Schedule, e = validateSchedule(in.Schedule)
	if e != nil {
		return in, e
	}
	if in.Minimum > in.Schedule.Target {
		return in, invalid("minimum", "Минимальный шаг не больше нормы")
	}
	if in.Kind == "binary" && (in.Minimum != 1 || math.Trunc(in.Schedule.Target) != in.Schedule.Target || (in.Schedule.Frequency == "daily" && in.Schedule.Target != 1)) {
		return in, invalid("schedule.target", "Бинарная привычка: один шаг; для недели/месяца — целое число выполнений")
	}
	if in.Kind == "duration" && in.Unit != "мин" {
		return in, invalid("unit", "Длительность измеряется в минутах")
	}
	start, e := date(in.StartsOn)
	if e != nil {
		return in, e
	}
	if in.EndsOn != nil {
		end, e := date(*in.EndsOn)
		if e != nil || end.Before(start) {
			return in, invalid("ends_on", "Конец не раньше начала")
		}
	}
	if in.AreaID != nil && !uuid(*in.AreaID) {
		return in, invalid("area_id", "Выберите направление")
	}
	return in, nil
}
func (s *Service) lock(ctx context.Context, tx pgx.Tx, id string) error {
	var v string
	if e := tx.QueryRow(ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, id).Scan(&v); e != nil {
		return e
	}
	if s.progression != nil {
		return s.progression.Lock(ctx, tx, id)
	}
	return nil
}
func (s *Service) area(ctx context.Context, tx pgx.Tx, id string, key *string) error {
	if key == nil {
		return nil
	}
	var ok bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM life_areas WHERE owner_id=$1 AND id=$2 AND NOT archived)`, id, *key).Scan(&ok)
	if e != nil {
		return e
	}
	if !ok {
		return invalid("area_id", "Направление не найдено")
	}
	return nil
}
func (s *Service) get(ctx context.Context, tx pgx.Tx, id, key string) (Habit, error) {
	h := Habit{AsOf: s.clock.Now().UTC(), Schedules: []ScheduleVersion{}, Periods: []Period{}}
	if !uuid(key) {
		return h, problem(404, "NOT_FOUND", "Привычка не найдена")
	}
	var data []byte
	e := tx.QueryRow(ctx, `SELECT id::text,payload,version,timezone,archived_at FROM habits WHERE owner_id=$1 AND id=$2`, id, key).Scan(&h.ID, &data, &h.Version, &h.Timezone, &h.ArchivedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return h, problem(404, "NOT_FOUND", "Привычка не найдена")
	}
	if e != nil {
		return h, e
	}
	if e = json.Unmarshal(data, &h.Input); e != nil {
		return h, e
	}
	rows, e := tx.Query(ctx, `SELECT version,effective_from::text,schedule,reason FROM habit_schedules WHERE owner_id=$1 AND habit_id=$2 ORDER BY effective_from,version`, id, key)
	if e != nil {
		return h, e
	}
	defer rows.Close()
	for rows.Next() {
		var v ScheduleVersion
		var b []byte
		if e = rows.Scan(&v.Version, &v.EffectiveFrom, &b, &v.Reason); e != nil {
			return h, e
		}
		if e = json.Unmarshal(b, &v.Schedule); e != nil {
			return h, e
		}
		h.Schedules = append(h.Schedules, v)
	}
	if e = rows.Err(); e != nil {
		return h, e
	}
	h.Pauses, e = s.pauses(ctx, tx, id, key)
	if e != nil {
		return h, e
	}
	h.Archives, e = s.archives(ctx, tx, id, key)
	return h, e
}
func (s *Service) create(ctx context.Context, tx pgx.Tx, id string, in Input) (Habit, error) {
	if in.StarterID != nil {
		var promoted *string
		e := tx.QueryRow(ctx, `SELECT promoted_entity_id::text FROM onboarding_starters WHERE owner_id=$1 AND id=$2 AND kind='habit' FOR UPDATE`, id, *in.StarterID).Scan(&promoted)
		if errors.Is(e, pgx.ErrNoRows) {
			return Habit{}, problem(404, "NOT_FOUND", "Стартовая привычка не найдена")
		}
		if e != nil {
			return Habit{}, e
		}
		if promoted != nil {
			return s.view(ctx, tx, id, *promoted)
		}
	}
	start, _ := date(in.StartsOn)
	now := s.clock.Now()
	if start.Before(now.AddDate(-10, 0, 0)) || start.After(now.AddDate(1, 0, 0)) {
		return Habit{}, invalid("starts_on", "Начало: не более 10 лет назад или одного года вперёд")
	}
	if e := s.area(ctx, tx, id, in.AreaID); e != nil {
		return Habit{}, e
	}
	var n int
	if e := tx.QueryRow(ctx, `SELECT count(*) FROM habits WHERE owner_id=$1`, id).Scan(&n); e != nil {
		return Habit{}, e
	}
	if n >= 100 {
		return Habit{}, invalid("title", "До 100 привычек, включая архив")
	}
	var zone, key string
	if e := tx.QueryRow(ctx, `SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1`, id).Scan(&zone); e != nil {
		return Habit{}, e
	}
	data, e := json.Marshal(in)
	if e != nil {
		return Habit{}, e
	}
	if e = tx.QueryRow(ctx, `INSERT INTO habits(owner_id,payload,timezone,created_at,updated_at,area_id) VALUES($1,$2,$3,$4,$4,$5) RETURNING id::text`, id, data, zone, s.clock.Now().UTC(), in.AreaID).Scan(&key); e != nil {
		return Habit{}, e
	}
	schedule, e := json.Marshal(in.Schedule)
	if e != nil {
		return Habit{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO habit_schedules(owner_id,habit_id,version,effective_from,schedule,reason) VALUES($1,$2,1,$3::text::date,$4,'Начальное расписание')`, id, key, in.StartsOn, schedule); e != nil {
		return Habit{}, e
	}
	if in.StarterID != nil {
		if _, e = tx.Exec(ctx, `UPDATE onboarding_starters SET promoted_entity_id=$3,version=version+1,updated_at=$4 WHERE owner_id=$1 AND id=$2`, id, *in.StarterID, key, s.clock.Now().UTC()); e != nil {
			return Habit{}, e
		}
	}
	return s.view(ctx, tx, id, key)
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
		if e := jobs.OwnerChanged(r.Context(), tx, id, s.clock); e != nil {
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
func (s *Service) reply(w http.ResponseWriter, r *http.Request, out any, e error) {
	if e != nil {
		var p *platform.APIError
		if errors.As(e, &p) {
			platform.WriteError(w, r, p)
			return
		}
		platform.WriteError(w, r, problem(500, "INTERNAL_ERROR", "Не удалось обработать привычку"))
		return
	}
	platform.WriteData(w, r, s.clock, 200, out)
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
func (s *Service) list(ctx context.Context, tx pgx.Tx, id string, archived bool) ([]Habit, error) {
	rows, e := tx.Query(ctx, `SELECT id::text FROM habits WHERE owner_id=$1 AND (archived_at IS NOT NULL)=$2 ORDER BY created_at,id`, id, archived)
	if e != nil {
		return nil, e
	}
	keys := []string{}
	for rows.Next() {
		var k string
		if e = rows.Scan(&k); e != nil {
			rows.Close()
			return nil, e
		}
		keys = append(keys, k)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	out := []Habit{}
	for _, k := range keys {
		v, e := s.view(ctx, tx, id, k)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	router.Group(func(r chi.Router) {
		r.Use(protect)
		s.mountSchedules(r)
		s.mountLogs(r)
		s.mountPauses(r)
		s.mountHistory(r)
		s.mountStarter(r)
		r.Get("/api/v1/habits", func(w http.ResponseWriter, r *http.Request) {
			id, e := owner(r.Context())
			var out []Habit
			if e == nil {
				e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
					if e := s.lock(r.Context(), tx, id); e != nil {
						return e
					}
					var err error
					out, err = s.list(r.Context(), tx, id, r.URL.Query().Get("archived") == "true")
					return err
				})
			}
			s.reply(w, r, out, e)
		})
		r.Get("/api/v1/habits/{id}", func(w http.ResponseWriter, r *http.Request) {
			id, e := owner(r.Context())
			var out Habit
			if e == nil {
				e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
					if e := s.lock(r.Context(), tx, id); e != nil {
						return e
					}
					var err error
					out, err = s.view(r.Context(), tx, id, chi.URLParam(r, "id"))
					return err
				})
			}
			s.reply(w, r, out, e)
		})
		r.Post("/api/v1/habits", func(w http.ResponseWriter, r *http.Request) {
			decode(s, w, r, func(in Input) (any, error) {
				valid, e := Validate(in)
				if e != nil {
					return nil, e
				}
				return s.mutate(r, valid, func(tx pgx.Tx, id string) (any, error) { return s.create(r.Context(), tx, id, valid) })
			})
		})
	})
}

func (s *Service) WithProgression(v *progression.Service) *Service { s.progression = v; return s }
