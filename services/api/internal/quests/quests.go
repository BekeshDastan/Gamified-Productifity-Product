package quests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
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

func NewService(pool *pgxpool.Pool, clock platform.Clock) *Service {
	return &Service{pool: pool, clock: clock}
}

type Input struct {
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	AreaID           *string    `json:"area_id"`
	Priority         string     `json:"priority"`
	Difficulty       int        `json:"difficulty"`
	Deadline         *time.Time `json:"deadline"`
	PlannedDate      *string    `json:"planned_date"`
	EstimatedMinutes int        `json:"estimated_minutes"`
	ActualMinutes    int        `json:"actual_minutes"`
	Tags             []string   `json:"tags"`
	Status           string     `json:"status"`
}
type Quest struct {
	Input
	ID          string     `json:"id"`
	Version     int64      `json:"version"`
	CompletedAt *time.Time `json:"completed_at"`
	ArchivedAt  *time.Time `json:"archived_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func problem(status int, code, message string) *platform.APIError {
	return &platform.APIError{Status: status, Code: code, Message: message}
}
func invalid(field, message string) *platform.APIError {
	p := problem(422, "VALIDATION_ERROR", "Проверьте поля задачи")
	p.Fields = map[string]string{field: message}
	return p
}
func uuid(value string) bool { var id pgtype.UUID; return id.Scan(value) == nil && id.Valid }
func owner(ctx context.Context) (string, error) {
	id, ok := platform.CurrentIdentity(ctx)
	if !ok {
		return "", problem(401, "UNAUTHORIZED", "Войдите в аккаунт")
	}
	return id.UserID, nil
}
func Validate(in Input) (Input, error) {
	in.Title = strings.TrimSpace(in.Title)
	if !utf8.ValidString(in.Title) || utf8.RuneCountInString(in.Title) < 1 || utf8.RuneCountInString(in.Title) > 200 || strings.IndexFunc(in.Title, unicode.IsControl) >= 0 {
		return in, invalid("title", "Название: от 1 до 200 символов")
	}
	if !utf8.ValidString(in.Description) || utf8.RuneCountInString(in.Description) > 10000 || strings.ContainsRune(in.Description, 0) {
		return in, invalid("description", "Описание: до 10000 символов")
	}
	if in.Priority != "low" && in.Priority != "normal" && in.Priority != "high" {
		return in, invalid("priority", "Выберите приоритет")
	}
	if in.Difficulty < 1 || in.Difficulty > 5 {
		return in, invalid("difficulty", "Сложность от 1 до 5")
	}
	switch in.Status {
	case "backlog", "todo", "in_progress", "postponed", "completed", "failed", "skipped":
	default:
		return in, invalid("status", "Выберите статус")
	}
	if in.PlannedDate != nil {
		v, e := time.Parse("2006-01-02", *in.PlannedDate)
		if e != nil || v.Format("2006-01-02") != *in.PlannedDate {
			return in, invalid("planned_date", "Укажите календарную дату")
		}
	}
	if in.Deadline != nil {
		v := in.Deadline.UTC()
		if v.Year() < 1 || v.Year() > 9999 {
			return in, invalid("deadline", "Укажите дату и время")
		}
		in.Deadline = &v
	}
	if in.EstimatedMinutes < 0 || in.EstimatedMinutes > 525600 {
		return in, invalid("estimated_minutes", "От 0 до 525600 минут")
	}
	if in.ActualMinutes < 0 || in.ActualMinutes > 525600 {
		return in, invalid("actual_minutes", "От 0 до 525600 минут")
	}
	if in.AreaID != nil && !uuid(*in.AreaID) {
		return in, invalid("area_id", "Выберите направление")
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	if len(in.Tags) > 50 {
		return in, invalid("tags", "Не более 50 тегов")
	}
	seen := map[string]bool{}
	for _, id := range in.Tags {
		if !uuid(id) || seen[id] {
			return in, invalid("tags", "Теги не должны повторяться")
		}
		seen[id] = true
	}
	return in, nil
}

const columns = `payload,id::text,status,version,completed_at,archived_at,created_at,updated_at`

func scan(row pgx.Row) (Quest, error) {
	var q Quest
	var data []byte
	var status string
	err := row.Scan(&data, &q.ID, &status, &q.Version, &q.CompletedAt, &q.ArchivedAt, &q.CreatedAt, &q.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(data, &q.Input)
	}
	q.Status = status
	return q, err
}
func (s *Service) lockOwner(ctx context.Context, tx pgx.Tx, id string) error {
	var ignored string
	return tx.QueryRow(ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, id).Scan(&ignored)
}
func get(ctx context.Context, tx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id, key string, lock bool) (Quest, error) {
	if !uuid(key) {
		return Quest{}, problem(404, "NOT_FOUND", "Задача не найдена")
	}
	sql := `SELECT ` + columns + ` FROM quests WHERE owner_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	q, err := scan(tx.QueryRow(ctx, sql, id, key))
	if errors.Is(err, pgx.ErrNoRows) {
		err = problem(404, "NOT_FOUND", "Задача не найдена")
	}
	return q, err
}
func (s *Service) labels(ctx context.Context, tx pgx.Tx, id string, in Input) error {
	if in.AreaID != nil {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM life_areas WHERE owner_id=$1 AND id=$2)`, id, *in.AreaID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return invalid("area_id", "Направление не найдено")
		}
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tags WHERE owner_id=$1 AND id=ANY($2::uuid[])`, id, in.Tags).Scan(&count); err != nil {
		return err
	}
	if count != len(in.Tags) {
		return invalid("tags", "Тег не найден")
	}
	return nil
}
func syncTags(ctx context.Context, tx pgx.Tx, id, key string, tags []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM quest_tags WHERE owner_id=$1 AND quest_id=$2`, id, key); err != nil {
		return err
	}
	for _, tag := range tags {
		if _, err := tx.Exec(ctx, `INSERT INTO quest_tags(owner_id,quest_id,tag_id) VALUES($1,$2,$3)`, id, key, tag); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) insert(ctx context.Context, tx pgx.Tx, id string, in Input) (Quest, error) {
	if err := s.labels(ctx, tx, id, in); err != nil {
		return Quest{}, err
	}
	data, err := json.Marshal(in)
	if err != nil {
		return Quest{}, err
	}
	q, err := scan(tx.QueryRow(ctx, `INSERT INTO quests(owner_id,area_id,payload,status,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$5) RETURNING `+columns, id, in.AreaID, data, in.Status, s.clock.Now().UTC()))
	if err == nil {
		err = syncTags(ctx, tx, id, q.ID, in.Tags)
	}
	if err == nil {
		err = s.event(ctx, tx, id, "created", "", nil, q)
	}
	return q, err
}
func (s *Service) Create(r *http.Request, in Input) (Quest, error) {
	id, err := owner(r.Context())
	if err != nil {
		return Quest{}, err
	}
	in, err = Validate(in)
	if err != nil {
		return Quest{}, err
	}
	if in.Status != "backlog" && in.Status != "todo" {
		return Quest{}, invalid("status", "Новая задача: Идея или К выполнению")
	}
	var q Quest
	err = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		if err := s.lockOwner(r.Context(), tx, id); err != nil {
			return err
		}
		var e error
		q, e = s.insert(r.Context(), tx, id, in)
		return e
	})
	return q, err
}
func (s *Service) Update(r *http.Request, key string, in Input) (Quest, error) {
	id, err := owner(r.Context())
	if err != nil {
		return Quest{}, err
	}
	in, err = Validate(in)
	if err != nil {
		return Quest{}, err
	}
	var q Quest
	err = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		if err := s.lockOwner(r.Context(), tx, id); err != nil {
			return err
		}
		current, err := get(r.Context(), tx, id, key, true)
		if err != nil {
			return err
		}
		if p := platform.MatchVersion(r, current.Version); p != nil {
			return p
		}
		if current.ArchivedAt != nil {
			return problem(409, "ARCHIVED", "Сначала восстановите задачу")
		}
		if current.Status != in.Status {
			return invalid("status", "Используйте действие изменения статуса")
		}
		if err := s.labels(r.Context(), tx, id, in); err != nil {
			return err
		}
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		q, err = scan(tx.QueryRow(r.Context(), `UPDATE quests SET payload=$3,area_id=$4,version=version+1,updated_at=$5 WHERE owner_id=$1 AND id=$2 RETURNING `+columns, id, key, data, in.AreaID, s.clock.Now().UTC()))
		if err == nil {
			err = syncTags(r.Context(), tx, id, key, in.Tags)
		}
		if err == nil {
			err = s.event(r.Context(), tx, id, "edited", "", &current, q)
		}
		return err
	})
	return q, err
}

type Page struct {
	Items    []Quest `json:"items"`
	Total    int     `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
}

func (s *Service) List(r *http.Request) (Page, error) {
	id, err := owner(r.Context())
	if err != nil {
		return Page{}, err
	}
	page, size := 1, 25
	for k, p := range map[string]*int{"page": &page, "page_size": &size} {
		if raw := r.URL.Query().Get(k); raw != "" {
			v, e := strconv.Atoi(raw)
			if e != nil || v < 1 || v > 10000 {
				return Page{}, invalid(k, "Некорректная страница")
			}
			*p = v
		}
	}
	if size > 100 {
		return Page{}, invalid("page_size", "Не более 100 записей")
	}
	filters, err := queryFilters(r.URL.Query())
	if err != nil {
		return Page{}, err
	}
	args := filters.args(id, s.clock.Now().UTC())
	result := Page{Items: []Quest{}, Page: page, PageSize: size}
	err = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
		if e := s.promoteStarter(r.Context(), tx, id); e != nil {
			return e
		}
		if e := tx.QueryRow(r.Context(), `SELECT count(*) FROM quests WHERE `+filteredWhere, args...).Scan(&result.Total); e != nil {
			return e
		}
		rows, e := tx.Query(r.Context(), `SELECT `+columns+` FROM quests WHERE `+filteredWhere+` ORDER BY `+sorts[filters.Sort]+` LIMIT $11 OFFSET $12`, append(args, size, (page-1)*size)...)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			q, e := scan(rows)
			if e != nil {
				return e
			}
			result.Items = append(result.Items, q)
		}
		return rows.Err()
	})
	return result, err
}

// Promotion uses the stable, locked source row; a reload cannot duplicate a draft.
func (s *Service) promoteStarter(ctx context.Context, tx pgx.Tx, id string) error {
	if err := s.lockOwner(ctx, tx, id); err != nil {
		return err
	}
	var key, title string
	err := tx.QueryRow(ctx, `SELECT id::text,title FROM onboarding_starters WHERE owner_id=$1 AND kind='task' AND promoted_entity_id IS NULL FOR UPDATE`, id).Scan(&key, &title)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	q, err := s.insert(ctx, tx, id, Input{Title: title, Priority: "normal", Difficulty: 1, Tags: []string{}, Status: "backlog"})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE onboarding_starters SET promoted_entity_id=$3,version=version+1,updated_at=$4 WHERE owner_id=$1 AND id=$2`, id, key, q.ID, s.clock.Now().UTC())
	return err
}
func (s *Service) reply(w http.ResponseWriter, r *http.Request, value any, err error) {
	if err != nil {
		var p *platform.APIError
		if !errors.As(err, &p) {
			p = problem(500, "INTERNAL_ERROR", "Не удалось выполнить запрос")
		}
		platform.WriteError(w, r, p)
		return
	}
	platform.WriteData(w, r, s.clock, 200, value)
}
func decode[T any](w http.ResponseWriter, r *http.Request, fn func(T) (any, error), s *Service) {
	in, p := platform.DecodeJSON[T](r)
	if p != nil {
		platform.WriteError(w, r, p)
		return
	}
	q, err := fn(in)
	s.reply(w, r, q, err)
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	router.Group(func(r chi.Router) {
		r.Use(protect)
		s.mountViews(r)
		s.mountLifecycle(r)
		r.Get("/api/v1/quests", func(w http.ResponseWriter, r *http.Request) { q, e := s.List(r); s.reply(w, r, q, e) })
		r.Post("/api/v1/quests", func(w http.ResponseWriter, r *http.Request) {
			decode(w, r, func(in Input) (any, error) { return s.Create(r, in) }, s)
		})
		r.Get("/api/v1/quests/{id}", func(w http.ResponseWriter, r *http.Request) {
			id, e := owner(r.Context())
			if e != nil {
				s.reply(w, r, nil, e)
				return
			}
			q, e := get(r.Context(), s.pool, id, chi.URLParam(r, "id"), false)
			s.reply(w, r, q, e)
		})
		r.Patch("/api/v1/quests/{id}", func(w http.ResponseWriter, r *http.Request) {
			decode(w, r, func(in Input) (any, error) { return s.Update(r, chi.URLParam(r, "id"), in) }, s)
		})
	})
}
