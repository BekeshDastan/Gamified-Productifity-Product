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
	"unicode"
	"unicode/utf8"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool  *pgxpool.Pool
	clock platform.Clock
}

func NewService(pool *pgxpool.Pool, clock platform.Clock) *Service { return &Service{pool, clock} }

type Input struct {
	Title        string  `json:"title"`
	Motive       string  `json:"motive"`
	AreaID       *string `json:"area_id"`
	StartDate    *string `json:"start_date"`
	TargetDate   *string `json:"target_date"`
	Criteria     string  `json:"criteria"`
	ProgressType string  `json:"progress_type"`
	Start        float64 `json:"start"`
	Target       float64 `json:"target"`
	Unit         string  `json:"unit"`
}
type Goal struct {
	Input
	ID         string     `json:"id"`
	State      string     `json:"state"`
	Progress   float64    `json:"progress"`
	Current    float64    `json:"current"`
	Version    int64      `json:"version"`
	Overdue    bool       `json:"overdue"`
	ArchivedAt *time.Time `json:"archived_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
type Create struct {
	Input
	State string `json:"state"`
}
type Edit struct {
	Input
	Reason string `json:"reason"`
}
type Page struct {
	Items    []Goal `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}
type reader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func problem(status int, code, message string) *platform.APIError {
	return &platform.APIError{Status: status, Code: code, Message: message}
}
func invalid(field, message string) *platform.APIError {
	p := problem(422, "VALIDATION_ERROR", "Проверьте поля цели")
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
func text(v string, max int) bool {
	return utf8.ValidString(v) && utf8.RuneCountInString(v) <= max && !strings.ContainsRune(v, 0)
}
func date(v *string) bool {
	if v == nil {
		return true
	}
	d, e := time.Parse("2006-01-02", *v)
	return e == nil && d.Year() > 0 && d.Format("2006-01-02") == *v
}
func amount(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1e9 && math.Abs(v*1000-math.Round(v*1000)) < 0.0001
}
func Validate(in Input) (Input, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Criteria = strings.TrimSpace(in.Criteria)
	in.Unit = strings.TrimSpace(in.Unit)
	if !text(in.Title, 200) || in.Title == "" || strings.IndexFunc(in.Title, unicode.IsControl) >= 0 {
		return in, invalid("title", "Название: 1–200 символов")
	}
	if !text(in.Motive, 10000) {
		return in, invalid("motive", "Мотив: до 10000 символов")
	}
	if !text(in.Criteria, 2000) || in.Criteria == "" {
		return in, invalid("criteria", "Опишите критерий успеха: 1–2000 символов")
	}
	if in.AreaID != nil && !uuid(*in.AreaID) {
		return in, invalid("area_id", "Выберите направление")
	}
	if !date(in.StartDate) || !date(in.TargetDate) || (in.StartDate != nil && in.TargetDate != nil && *in.TargetDate < *in.StartDate) {
		return in, invalid("target_date", "Целевая дата не раньше начальной")
	}
	switch in.ProgressType {
	case "milestones", "quests", "metric":
	default:
		return in, invalid("progress_type", "Выберите способ расчёта")
	}
	if !amount(in.Start) || !amount(in.Target) || in.Target <= in.Start {
		return in, invalid("target", "Целевое значение должно быть больше начального; до 3 знаков после запятой")
	}
	if !text(in.Unit, 40) || strings.IndexFunc(in.Unit, unicode.IsControl) >= 0 || (in.ProgressType == "metric" && in.Unit == "") {
		return in, invalid("unit", "Укажите единицу: до 40 символов")
	}
	if in.ProgressType != "metric" && (in.Start != 0 || in.Target != 1 || in.Unit != "") {
		return in, invalid("progress_type", "Числовые параметры относятся к числовой цели")
	}
	return in, nil
}

const columns = `g.id::text,g.payload,g.state,g.progress::float8,g.version,g.archived_at,g.created_at,g.updated_at,g.current::float8,
 COALESCE(g.target_date < ($3::timestamptz AT TIME ZONE COALESCE((SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=g.owner_id),'UTC'))::date AND g.state IN('draft','active','paused'),false)`

func scan(row pgx.Row) (Goal, error) {
	var g Goal
	var data []byte
	e := row.Scan(&g.ID, &data, &g.State, &g.Progress, &g.Version, &g.ArchivedAt, &g.CreatedAt, &g.UpdatedAt, &g.Current, &g.Overdue)
	if e == nil {
		e = json.Unmarshal(data, &g.Input)
	}
	return g, e
}
func (s *Service) lock(ctx context.Context, tx pgx.Tx, id string) error {
	var v string
	return tx.QueryRow(ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, id).Scan(&v)
}
func (s *Service) get(ctx context.Context, q reader, id, key string) (Goal, error) {
	if !uuid(key) {
		return Goal{}, problem(404, "NOT_FOUND", "Цель не найдена")
	}
	g, e := scan(q.QueryRow(ctx, `SELECT `+columns+` FROM goals g WHERE g.owner_id=$1 AND g.id=$2`, id, key, s.clock.Now().UTC()))
	if errors.Is(e, pgx.ErrNoRows) {
		e = problem(404, "NOT_FOUND", "Цель не найдена")
	}
	return g, e
}
func (s *Service) labels(ctx context.Context, tx pgx.Tx, id string, in Input) error {
	if in.AreaID == nil {
		return nil
	}
	var exists bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM life_areas WHERE owner_id=$1 AND id=$2)`, id, *in.AreaID).Scan(&exists)
	if e == nil && !exists {
		return invalid("area_id", "Направление не найдено")
	}
	return e
}
func (s *Service) event(ctx context.Context, tx pgx.Tx, id, kind, reason string, g Goal) error {
	data, e := s.snapshot(ctx, tx, id, g)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO goal_versions(owner_id,goal_id,goal_version,kind,reason,snapshot,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, g.ID, g.Version, kind, reason, data, s.clock.Now().UTC())
	if e != nil {
		return e
	}
	return jobs.OwnerChanged(ctx, tx, id, s.clock)
}
func (s *Service) insert(ctx context.Context, tx pgx.Tx, id string, in Create) (Goal, error) {
	if e := s.labels(ctx, tx, id, in.Input); e != nil {
		return Goal{}, e
	}
	data, e := json.Marshal(in.Input)
	if e != nil {
		return Goal{}, e
	}
	var key string
	e = tx.QueryRow(ctx, `INSERT INTO goals(owner_id,area_id,payload,state,start_date,target_date,created_at,updated_at) VALUES($1,$2,$3,$4,$5::text::date,$6::text::date,$7,$7) RETURNING id::text`, id, in.AreaID, data, in.State, in.StartDate, in.TargetDate, s.clock.Now().UTC()).Scan(&key)
	if e != nil {
		return Goal{}, e
	}
	if e = s.setupMetric(ctx, tx, id, key, in.Input); e != nil {
		return Goal{}, e
	}
	if _, e = tx.Exec(ctx, `UPDATE goals SET current=$3 WHERE owner_id=$1 AND id=$2`, id, key, in.Start); e != nil {
		return Goal{}, e
	}
	g, e := s.get(ctx, tx, id, key)
	if e == nil {
		e = s.event(ctx, tx, id, "created", "", g)
	}
	return g, e
}
func (s *Service) promote(ctx context.Context, tx pgx.Tx, id string) error {
	if e := s.lock(ctx, tx, id); e != nil {
		return e
	}
	var key, title string
	e := tx.QueryRow(ctx, `SELECT id::text,title FROM onboarding_starters WHERE owner_id=$1 AND kind='goal' AND promoted_entity_id IS NULL FOR UPDATE`, id).Scan(&key, &title)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	g, e := s.insert(ctx, tx, id, Create{Input: Input{Title: title, Criteria: "Уточнить критерий достижения", ProgressType: "milestones", Target: 1}, State: "draft"})
	if e == nil {
		_, e = tx.Exec(ctx, `UPDATE onboarding_starters SET promoted_entity_id=$3,version=version+1,updated_at=$4 WHERE owner_id=$1 AND id=$2`, id, key, g.ID, s.clock.Now().UTC())
	}
	return e
}
func (s *Service) list(r *http.Request) (Page, error) {
	id, e := owner(r.Context())
	if e != nil {
		return Page{}, e
	}
	v := r.URL.Query()
	state, area, search := v.Get("state"), v.Get("area_id"), strings.TrimSpace(v.Get("search"))
	if state != "" && !strings.Contains("|draft|active|paused|achieved|not_achieved|cancelled|", "|"+state+"|") {
		return Page{}, invalid("state", "Выберите состояние")
	}
	if (area != "" && !uuid(area)) || !text(search, 200) {
		return Page{}, invalid("search", "Некорректный фильтр")
	}
	archived := v.Get("archived") == "true"
	if v.Get("archived") != "" && v.Get("archived") != "true" && v.Get("archived") != "false" {
		return Page{}, invalid("archived", "Укажите true/false")
	}
	page, size := 1, 25
	for k, p := range map[string]*int{"page": &page, "page_size": &size} {
		if raw := v.Get(k); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil || n < 1 || n > 10000 {
				return Page{}, invalid(k, "Некорректная страница")
			}
			*p = n
		}
	}
	if size > 100 {
		return Page{}, invalid("page_size", "Не более 100")
	}
	out := Page{Items: []Goal{}, Page: page, PageSize: size}
	e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		if e := s.promote(r.Context(), tx, id); e != nil {
			return e
		}
		where := `g.owner_id=$1 AND ($4='' OR g.state=$4) AND ($5='' OR g.area_id::text=$5) AND (g.archived_at IS NOT NULL)=$6 AND ($7='' OR strpos(lower(g.payload->>'title'),lower($7))>0)`
		args := []any{id, "", s.clock.Now().UTC(), state, area, archived, search}
		// Count uses the same typed parameters as the result query.
		if e := tx.QueryRow(r.Context(), `SELECT count(*) FROM goals g WHERE `+where+` AND $2::text='' AND $3::timestamptz IS NOT NULL`, args...).Scan(&out.Total); e != nil {
			return e
		}
		rows, e := tx.Query(r.Context(), `SELECT `+columns+` FROM goals g WHERE `+where+` AND $2::text='' ORDER BY g.created_at DESC,g.id DESC LIMIT $8 OFFSET $9`, append(args, size, (page-1)*size)...)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			g, e := scan(rows)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, g)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) reply(w http.ResponseWriter, r *http.Request, v any, e error) {
	if e != nil {
		var p *platform.APIError
		if !errors.As(e, &p) {
			p = problem(500, "INTERNAL_ERROR", "Не удалось выполнить запрос")
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
		s.mountPlan(r)
		s.mountMetrics(r)
		s.mountTemplates(r)
		s.mountLifecycle(r)
		r.Get("/api/v1/goals", func(w http.ResponseWriter, r *http.Request) { v, e := s.list(r); s.reply(w, r, v, e) })
		r.Get("/api/v1/goals/{id}", func(w http.ResponseWriter, r *http.Request) {
			id, e := owner(r.Context())
			if e != nil {
				s.reply(w, r, nil, e)
				return
			}
			v, e := s.get(r.Context(), s.pool, id, chi.URLParam(r, "id"))
			s.reply(w, r, v, e)
		})
		r.Post("/api/v1/goals", func(w http.ResponseWriter, r *http.Request) {
			decode(s, w, r, func(in Create) (any, error) {
				id, e := owner(r.Context())
				if e != nil {
					return nil, e
				}
				in.Input, e = Validate(in.Input)
				if e != nil {
					return nil, e
				}
				if in.State != "draft" && in.State != "active" {
					return nil, invalid("state", "Новая цель: черновик или активная")
				}
				var g Goal
				e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
					if e := s.lock(r.Context(), tx, id); e != nil {
						return e
					}
					g, e = s.insert(r.Context(), tx, id, in)
					return e
				})
				return g, e
			})
		})
		r.Patch("/api/v1/goals/{id}", func(w http.ResponseWriter, r *http.Request) {
			decode(s, w, r, func(in Edit) (any, error) {
				id, e := owner(r.Context())
				if e != nil {
					return nil, e
				}
				in.Input, e = Validate(in.Input)
				if e != nil {
					return nil, e
				}
				var g Goal
				e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
					if e := s.lock(r.Context(), tx, id); e != nil {
						return e
					}
					g, e = s.get(r.Context(), tx, id, chi.URLParam(r, "id"))
					if e != nil {
						return e
					}
					if p := platform.MatchVersion(r, g.Version); p != nil {
						return p
					}
					if g.ArchivedAt != nil || (g.State != "draft" && g.State != "active" && g.State != "paused") {
						return problem(409, "INVALID_TRANSITION", "Для изменения сначала пересмотрите цель")
					}
					if g.State != "draft" && strings.TrimSpace(in.Reason) == "" {
						return invalid("reason", "Укажите причину пересмотра критериев или плана")
					}
					if !text(in.Reason, 1000) {
						return invalid("reason", "До 1000 символов")
					}
					if e := s.labels(r.Context(), tx, id, in.Input); e != nil {
						return e
					}
					if e := s.setupMetric(r.Context(), tx, id, g.ID, in.Input); e != nil {
						return e
					}
					data, e := json.Marshal(in.Input)
					if e != nil {
						return e
					}
					_, e = tx.Exec(r.Context(), `UPDATE goals SET payload=$3,area_id=$4,start_date=$5::text::date,target_date=$6::text::date,updated_at=$7 WHERE owner_id=$1 AND id=$2`, id, g.ID, data, in.AreaID, in.StartDate, in.TargetDate, s.clock.Now().UTC())
					if e != nil {
						return e
					}
					g, e = s.get(r.Context(), tx, id, g.ID)
					if e == nil {
						g, e = s.touch(r.Context(), tx, id, g, "edited", in.Reason)
					}
					return e
				})
				return g, e
			})
		})
	})
}
