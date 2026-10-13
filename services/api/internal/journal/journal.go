package journal

import (
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Service struct {
	*privateapi.Service
	plans *planning.Service
}

func NewService(pool *pgxpool.Pool, clock platform.Clock) *Service {
	return &Service{Service: privateapi.New(pool, clock)}
}

type Input struct {
	Date     string   `json:"date"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Tags     []string `json:"tags"`
	GoalID   *string  `json:"goal_id"`
	Archived bool     `json:"archived"`
}
type Entry struct {
	Input
	ID        string    `json:"id"`
	Version   int64     `json:"version"`
	HTML      string    `json:"html"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func validate(in Input) (Input, error) {
	in.Title = strings.TrimSpace(in.Title)
	if e := privateapi.Day(in.Date); e != nil {
		return in, e
	}
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 200 || strings.IndexFunc(in.Title, unicode.IsControl) >= 0 || utf8.RuneCountInString(in.Body) > 50000 || strings.ContainsRune(in.Body, 0) || len(in.Tags) > 20 {
		return in, privateapi.Invalid("Название до 200, текст до 50000 символов, до 20 тегов")
	}
	if in.GoalID != nil && !privateapi.UUID(*in.GoalID) {
		return in, privateapi.Invalid("Выберите цель")
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	seen := map[string]bool{}
	for i, tag := range in.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || utf8.RuneCountInString(tag) > 50 || strings.IndexFunc(tag, unicode.IsControl) >= 0 || seen[tag] {
			return in, privateapi.Invalid("Теги: разные названия до 50 символов")
		}
		seen[tag] = true
		in.Tags[i] = tag
	}
	return in, nil
}

const columns = "id::text,local_date::text,title,body,tags,goal_id::text,archived,version,created_at,updated_at"

func scan(row pgx.Row) (Entry, error) {
	var out Entry
	e := row.Scan(&out.ID, &out.Date, &out.Title, &out.Body, &out.Tags, &out.GoalID, &out.Archived, &out.Version, &out.CreatedAt, &out.UpdatedAt)
	out.HTML = RenderMarkdown(out.Body)
	if errors.Is(e, pgx.ErrNoRows) {
		e = privateapi.Problem(404, "NOT_FOUND", "Запись не найдена")
	}
	return out, e
}
func (s *Service) get(ctx context.Context, tx pgx.Tx, owner, id string) (Entry, error) {
	if !privateapi.UUID(id) {
		return Entry{}, privateapi.Problem(404, "NOT_FOUND", "Запись не найдена")
	}
	return scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM journal_entries WHERE owner_id=$1 AND id=$2", owner, id))
}
func (s *Service) save(r *http.Request, tx pgx.Tx, owner, id string, in Input) (Entry, error) {
	ctx := r.Context()
	in, e := validate(in)
	if e != nil {
		return Entry{}, e
	}
	if in.GoalID != nil {
		var exists bool
		e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM goals WHERE owner_id=$1 AND id=$2)", owner, *in.GoalID).Scan(&exists)
		if e != nil {
			return Entry{}, e
		}
		if !exists {
			return Entry{}, privateapi.Problem(404, "NOT_FOUND", "Цель не найдена")
		}
	}
	now := s.Clock.Now().UTC()
	if id == "" {
		return scan(tx.QueryRow(ctx, "INSERT INTO journal_entries(owner_id,local_date,title,body,tags,goal_id,archived,created_at,updated_at) VALUES($1,$2::text::date,$3,$4,$5,$6,$7,$8,$8) RETURNING "+columns, owner, in.Date, in.Title, in.Body, in.Tags, in.GoalID, in.Archived, now))
	}
	old, e := s.get(ctx, tx, owner, id)
	if e != nil {
		return old, e
	}
	if p := platform.MatchVersion(r, old.Version); p != nil {
		return old, p
	}
	return scan(tx.QueryRow(ctx, "UPDATE journal_entries SET local_date=$3::text::date,title=$4,body=$5,tags=$6,goal_id=$7,archived=$8,version=version+1,updated_at=$9 WHERE owner_id=$1 AND id=$2 RETURNING "+columns, owner, id, in.Date, in.Title, in.Body, in.Tags, in.GoalID, in.Archived, now))
}

type Page struct {
	Items []Entry `json:"items"`
	Next  string  `json:"next"`
}

func (s *Service) list(r *http.Request, tx pgx.Tx, owner string) (Page, error) {
	q := r.URL.Query()
	after := q.Get("after")
	if after == "" {
		after = "00000000-0000-0000-0000-000000000000"
	}
	if !privateapi.UUID(after) {
		return Page{}, privateapi.Invalid("Некорректная страница")
	}
	from, to := q.Get("from"), q.Get("to")
	if from == "" {
		from = "0001-01-01"
	}
	if to == "" {
		to = "9998-12-31"
	}
	if privateapi.Day(from) != nil || privateapi.Day(to) != nil || from > to {
		return Page{}, privateapi.Invalid("Проверьте диапазон дат")
	}
	tag := q.Get("tag")
	if utf8.RuneCountInString(tag) > 50 {
		return Page{}, privateapi.Invalid("Тег до 50 символов")
	}
	rows, e := tx.Query(r.Context(), "SELECT "+columns+" FROM journal_entries WHERE owner_id=$1 AND id>$2 AND local_date BETWEEN $3::text::date AND $4::text::date AND archived=$5 AND ($6='' OR $6=ANY(tags)) ORDER BY id LIMIT 26", owner, after, from, to, q.Get("archived") == "true", tag)
	if e != nil {
		return Page{}, e
	}
	defer rows.Close()
	out := Page{Items: []Entry{}}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > 25 {
		out.Items = out.Items[:25]
		out.Next = out.Items[24].ID
	}
	return out, nil
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	router.Group(func(r chi.Router) {
		r.Use(protect)
		s.mountMorning(r)
		s.mountEvening(r)
		r.Get("/api/v1/journal/entries", func(w http.ResponseWriter, r *http.Request) {
			out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) { return s.list(r, tx, owner) })
			s.Reply(w, r, out, e)
		})
		r.Get("/api/v1/journal/entries/{id}", func(w http.ResponseWriter, r *http.Request) {
			out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.get(ctx, tx, owner, chi.URLParam(r, "id"))
			})
			s.Reply(w, r, out, e)
		})
		for _, method := range []string{"POST", "PUT"} {
			path := "/api/v1/journal/entries"
			if method == "PUT" {
				path += "/{id}"
			}
			r.MethodFunc(method, path, func(w http.ResponseWriter, r *http.Request) {
				privateapi.Decode(s.Service, w, r, func(in Input) (any, error) {
					return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
						return s.save(r, tx, owner, chi.URLParam(r, "id"), in)
					})
				})
			})
		}
	})
}
