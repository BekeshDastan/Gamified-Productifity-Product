package quests

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type Filters struct {
	Search      string `json:"search"`
	Status      string `json:"status"`
	AreaID      string `json:"area_id"`
	Priority    string `json:"priority"`
	PlannedFrom string `json:"planned_from"`
	PlannedTo   string `json:"planned_to"`
	Overdue     bool   `json:"overdue"`
	Archived    bool   `json:"archived"`
	Sort        string `json:"sort"`
}

func validateFilters(f Filters) (Filters, error) {
	f.Search = strings.TrimSpace(f.Search)
	if !utf8.ValidString(f.Search) || utf8.RuneCountInString(f.Search) > 200 {
		return f, invalid("search", "До 200 символов")
	}
	if f.Status != "" {
		if _, e := Validate(Input{Title: "x", Priority: "normal", Difficulty: 1, Status: f.Status}); e != nil {
			return f, invalid("status", "Неизвестный статус")
		}
	}
	if f.AreaID != "" && !uuid(f.AreaID) {
		return f, invalid("area_id", "Укажите направление")
	}
	if f.Priority != "" && f.Priority != "low" && f.Priority != "normal" && f.Priority != "high" {
		return f, invalid("priority", "Укажите приоритет")
	}
	for _, v := range []string{f.PlannedFrom, f.PlannedTo} {
		if v != "" {
			if _, e := time.Parse("2006-01-02", v); e != nil {
				return f, invalid("planned_from", "Укажите дату")
			}
		}
	}
	if f.PlannedFrom != "" && f.PlannedTo != "" && f.PlannedFrom > f.PlannedTo {
		return f, invalid("planned_to", "Конец диапазона раньше начала")
	}
	if f.Sort == "" {
		f.Sort = "newest"
	}
	if _, ok := sorts[f.Sort]; !ok {
		return f, invalid("sort", "Неизвестная сортировка")
	}
	return f, nil
}

var sorts = map[string]string{
	"newest": "created_at DESC,id DESC", "oldest": "created_at,id",
	"title":    "lower(payload->>'title'),id",
	"planned":  "payload->>'planned_date' ASC NULLS LAST,id",
	"priority": "CASE payload->>'priority' WHEN 'high' THEN 0 WHEN 'normal' THEN 1 ELSE 2 END,id",
}

func queryFilters(q url.Values) (Filters, error) {
	for _, name := range []string{"overdue", "archived"} {
		if q.Get(name) != "" && q.Get(name) != "true" && q.Get(name) != "false" {
			return Filters{}, invalid(name, "Укажите true или false")
		}
	}
	return validateFilters(Filters{Search: q.Get("search"), Status: q.Get("status"), AreaID: q.Get("area_id"), Priority: q.Get("priority"), PlannedFrom: q.Get("planned_from"), PlannedTo: q.Get("planned_to"), Overdue: q.Get("overdue") == "true", Archived: q.Get("archived") == "true", Sort: q.Get("sort")})
}

// Ordering comes only from the allowlist; every client value remains a parameter.
const filteredWhere = `owner_id=$1 AND (archived_at IS NOT NULL)=$2
 AND ($3='' OR strpos(lower((payload->>'title')||' '||(payload->>'description')),lower($3))>0)
 AND ($4='' OR status=$4) AND ($5='' OR area_id::text=$5)
 AND ($6='' OR payload->>'priority'=$6)
 AND ($7='' OR payload->>'planned_date'>=$7) AND ($8='' OR payload->>'planned_date'<=$8)
 AND (NOT $9 OR (status NOT IN ('completed','failed','skipped') AND (payload->>'deadline')::timestamptz<$10))`

func (f Filters) args(id string, now time.Time) []any {
	return []any{id, f.Archived, f.Search, f.Status, f.AreaID, f.Priority, f.PlannedFrom, f.PlannedTo, f.Overdue, now}
}

type ViewInput struct {
	Name    string  `json:"name"`
	Filters Filters `json:"filters"`
}
type View struct {
	ViewInput
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

func (s *Service) mountViews(r chi.Router) {
	r.Get("/api/v1/quests/views", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		rows, e := s.pool.Query(r.Context(), `SELECT id::text,name,filters,version FROM quest_views WHERE owner_id=$1 ORDER BY name,id`, id)
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		defer rows.Close()
		out := []View{}
		for rows.Next() {
			var v View
			var data []byte
			e = rows.Scan(&v.ID, &v.Name, &data, &v.Version)
			if e == nil {
				e = json.Unmarshal(data, &v.Filters)
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
	r.Post("/api/v1/quests/views", func(w http.ResponseWriter, r *http.Request) {
		decode(w, r, func(in ViewInput) (any, error) {
			id, e := owner(r.Context())
			if e != nil {
				return nil, e
			}
			in.Name = strings.TrimSpace(in.Name)
			if in.Name == "" || utf8.RuneCountInString(in.Name) > 60 {
				return nil, invalid("name", "Название: 1–60 символов")
			}
			in.Filters, e = validateFilters(in.Filters)
			if e != nil {
				return nil, e
			}
			data, e := json.Marshal(in.Filters)
			if e != nil {
				return nil, e
			}
			var v View
			v.ViewInput = in
			e = s.pool.QueryRow(r.Context(), `INSERT INTO quest_views(owner_id,name,filters,created_at) SELECT $1,$2,$3,$4 WHERE (SELECT count(*) FROM quest_views WHERE owner_id=$1)<50 ON CONFLICT(owner_id,name) DO NOTHING RETURNING id::text,version`, id, in.Name, data, s.clock.Now().UTC()).Scan(&v.ID, &v.Version)
			if e != nil {
				return nil, invalid("name", "Название занято или достигнут лимит 50 представлений")
			}
			return v, nil
		}, s)
	})
	r.Delete("/api/v1/quests/views/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		key := chi.URLParam(r, "id")
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		if !uuid(key) {
			s.reply(w, r, nil, problem(404, "NOT_FOUND", "Не найдено"))
			return
		}
		var version int64
		e = s.pool.QueryRow(r.Context(), `SELECT version FROM quest_views WHERE owner_id=$1 AND id=$2`, id, key).Scan(&version)
		if e != nil {
			s.reply(w, r, nil, problem(404, "NOT_FOUND", "Не найдено"))
			return
		}
		if p := platform.MatchVersion(r, version); p != nil {
			s.reply(w, r, nil, p)
			return
		}
		_, e = s.pool.Exec(r.Context(), `DELETE FROM quest_views WHERE owner_id=$1 AND id=$2 AND version=$3`, id, key, version)
		s.reply(w, r, struct{}{}, e)
	})
}
