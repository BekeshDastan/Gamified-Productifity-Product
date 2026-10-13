package quests

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func (s *Service) archiveTx(r *http.Request, tx pgx.Tx, id, key string, restore bool, reasons ...string) (Quest, error) {
	if e := s.lockOwner(r.Context(), tx, id); e != nil {
		return Quest{}, e
	}
	q, e := get(r.Context(), tx, id, key, true)
	if e != nil {
		return q, e
	}
	if p := platform.MatchVersion(r, q.Version); p != nil {
		return q, p
	}
	if (restore && q.ArchivedAt == nil) || (!restore && q.ArchivedAt != nil) {
		return q, problem(409, "INVALID_TRANSITION", "Задача уже в выбранном состоянии архива")
	}
	old := q
	kind := "archive"
	q.ArchivedAt = nil
	if !restore {
		now := s.clock.Now().UTC()
		q.ArchivedAt = &now
	} else {
		kind = "restore"
	}
	q, e = scan(tx.QueryRow(r.Context(), `UPDATE quests SET archived_at=$3,version=version+1,updated_at=$4 WHERE owner_id=$1 AND id=$2 RETURNING `+columns, id, key, q.ArchivedAt, s.clock.Now().UTC()))
	if e == nil {
		why := ""
		if len(reasons) > 0 {
			why = reasons[0]
		}
		e = s.event(r.Context(), tx, id, kind, why, &old, &q)
	}
	return q, e
}

type BatchItem struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}
type Batch struct {
	Items      []BatchItem `json:"items"`
	Action     string      `json:"action"`
	Transition Transition  `json:"transition"`
}

var errPreview = errors.New("rollback batch preview")

func (s *Service) batchTx(r *http.Request, tx pgx.Tx, in Batch) ([]Quest, error) {
	id, e := owner(r.Context())
	if e != nil {
		return nil, e
	}
	if e := s.lockOwner(r.Context(), tx, id); e != nil {
		return nil, e
	}
	if len(in.Items) < 1 || len(in.Items) > 50 {
		return nil, invalid("items", "Выберите от 1 до 50 задач")
	}
	if in.Action != "archive" && in.Action != "restore" {
		if _, e := nextStatus("todo", in.Action); e != nil && in.Action != "ready" && in.Action != "reopen" {
			return nil, invalid("action", "Выберите пакетное действие")
		}
	}
	seen := map[string]bool{}
	original := map[string]Quest{}
	type ordered struct {
		item  BatchItem
		depth int
	}
	order := make([]ordered, 0, len(in.Items))
	for _, item := range in.Items {
		if !uuid(item.ID) || item.Version < 1 || seen[item.ID] {
			return nil, invalid("items", "Задачи должны быть разными и иметь версию")
		}
		seen[item.ID] = true
		q, e := get(r.Context(), tx, id, item.ID, true)
		if e != nil {
			return nil, e
		}
		request := r.Clone(r.Context())
		request.Header = r.Header.Clone()
		request.Header.Set("If-Match", strconv.Quote(strconv.FormatInt(item.Version, 10)))
		if p := platform.MatchVersion(request, q.Version); p != nil {
			return nil, p
		}
		if in.Action != "archive" && in.Action != "restore" {
			if _, e := nextStatus(q.Status, in.Action); e != nil {
				return nil, e
			}
		}
		original[q.ID] = q
		depth := 0
		parent := q.ParentID
		for parent != nil {
			p, e := get(r.Context(), tx, id, *parent, false)
			if e != nil {
				return nil, e
			}
			depth++
			parent = p.ParentID
		}
		order = append(order, ordered{item, depth})
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].depth != order[j].depth {
			return order[i].depth > order[j].depth
		}
		return order[i].item.ID < order[j].item.ID
	})
	out := make([]Quest, 0, len(order))
	for _, entry := range order {
		q, e := get(r.Context(), tx, id, entry.item.ID, true)
		if e != nil {
			return nil, e
		}
		request := r.Clone(r.Context())
		request.Header = r.Header.Clone()
		request.Header.Set("If-Match", strconv.Quote(strconv.FormatInt(q.Version, 10)))
		if in.Action == "archive" || in.Action == "restore" {
			q, e = s.archiveTx(request, tx, id, q.ID, in.Action == "restore")
		} else {
			target, _ := nextStatus(original[q.ID].Status, in.Action)
			// An earlier child in this same atomic batch may already roll up its parent.
			if q.Version == original[q.ID].Version || q.Status != target {
				q, e = s.transitionTx(request, tx, id, q.ID, in.Action, in.Transition)
			}
		}
		if e != nil {
			return nil, e
		}
		out = append(out, q)
	}
	return out, nil
}

type Template struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Input   Input  `json:"input"`
	Version int64  `json:"version"`
}
type TemplateSource struct {
	Name    string `json:"name"`
	QuestID string `json:"quest_id"`
}
type TemplateInput struct {
	Name  string `json:"name"`
	Input Input  `json:"input"`
}
type TemplateDates struct {
	PlannedDate *string    `json:"planned_date"`
	Deadline    *time.Time `json:"deadline"`
}
type TemplatePreview struct {
	Input   Input `json:"input"`
	Version int64 `json:"version"`
}

func templateName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) < 1 || utf8.RuneCountInString(value) > 80 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", invalid("name", "Название шаблона: 1–80 символов")
	}
	return value, nil
}
func reusable(in Input) Input {
	in.ParentID = nil
	in.Status = "backlog"
	in.ActualMinutes = 0
	in.ActualTimeMode = "from_focus"
	in.PlannedDate = nil
	in.Deadline = nil
	return in
}
func template(ctx context.Context, tx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id, key string) (Template, error) {
	if !uuid(key) {
		return Template{}, problem(404, "NOT_FOUND", "Шаблон не найден")
	}
	var t Template
	var data []byte
	e := tx.QueryRow(ctx, `SELECT id::text,name,payload,version FROM quest_templates WHERE owner_id=$1 AND id=$2`, id, key).Scan(&t.ID, &t.Name, &data, &t.Version)
	if errors.Is(e, pgx.ErrNoRows) {
		e = problem(404, "NOT_FOUND", "Шаблон не найден")
	}
	if e == nil {
		e = json.Unmarshal(data, &t.Input)
	}
	return t, e
}
func templateError(e error) error {
	var db *pgconn.PgError
	if errors.As(e, &db) && db.Code == "23505" {
		return invalid("name", "Название шаблона уже занято")
	}
	return e
}
func (s *Service) templatePreview(ctx context.Context, tx pgx.Tx, id, key string, dates TemplateDates) (TemplatePreview, error) {
	t, e := template(ctx, tx, id, key)
	if e != nil {
		return TemplatePreview{}, e
	}
	in := reusable(t.Input)
	in.PlannedDate = dates.PlannedDate
	in.Deadline = dates.Deadline
	in, e = Validate(in)
	if e == nil {
		e = s.labels(ctx, tx, id, in)
	}
	return TemplatePreview{Input: in, Version: t.Version}, e
}
func (s *Service) mountTools(r chi.Router) {
	for _, action := range []string{"archive", "restore"} {
		r.Post("/api/v1/quests/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			decode(w, r, func(in struct{}) (any, error) {
				id, e := owner(r.Context())
				if e != nil {
					return nil, e
				}
				return s.idempotent(r, in, func(tx pgx.Tx) (any, error) {
					return s.archiveTx(r, tx, id, chi.URLParam(r, "id"), action == "restore")
				})
			}, s)
		})
	}
	r.Post("/api/v1/quests/batch/preview", func(w http.ResponseWriter, r *http.Request) {
		decode(w, r, func(in Batch) (any, error) {
			var out []Quest
			e := storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
				var e error
				out, e = s.batchTx(r, tx, in)
				if e != nil {
					return e
				}
				return errPreview
			})
			if errors.Is(e, errPreview) {
				e = nil
			}
			return out, e
		}, s)
	})
	r.Post("/api/v1/quests/batch", func(w http.ResponseWriter, r *http.Request) {
		decode(w, r, func(in Batch) (any, error) {
			return s.idempotent(r, in, func(tx pgx.Tx) (any, error) { return s.batchTx(r, tx, in) })
		}, s)
	})
	r.Get("/api/v1/templates/quests", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		rows, e := s.pool.Query(r.Context(), `SELECT id::text,name,payload,version FROM quest_templates WHERE owner_id=$1 ORDER BY lower(name),id`, id)
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		defer rows.Close()
		out := []Template{}
		for rows.Next() {
			var t Template
			var data []byte
			e = rows.Scan(&t.ID, &t.Name, &data, &t.Version)
			if e == nil {
				e = json.Unmarshal(data, &t.Input)
			}
			if e != nil {
				break
			}
			out = append(out, t)
		}
		if e == nil {
			e = rows.Err()
		}
		s.reply(w, r, out, e)
	})
	r.Post("/api/v1/templates/quests", func(w http.ResponseWriter, r *http.Request) {
		decode(w, r, func(in TemplateSource) (any, error) {
			id, e := owner(r.Context())
			if e != nil {
				return nil, e
			}
			name, e := templateName(in.Name)
			if e != nil {
				return nil, e
			}
			var t Template
			e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
				if e := s.lockOwner(r.Context(), tx, id); e != nil {
					return e
				}
				q, e := get(r.Context(), tx, id, in.QuestID, false)
				if e != nil {
					return e
				}
				var count int
				if e := tx.QueryRow(r.Context(), `SELECT count(*) FROM quest_templates WHERE owner_id=$1`, id).Scan(&count); e != nil {
					return e
				}
				if count >= 100 {
					return invalid("name", "Не более 100 шаблонов")
				}
				t.Name = name
				t.Input = reusable(q.Input)
				data, e := json.Marshal(t.Input)
				if e != nil {
					return e
				}
				return tx.QueryRow(r.Context(), `INSERT INTO quest_templates(owner_id,name,payload,created_at,updated_at) VALUES($1,$2,$3,$4,$4) RETURNING id::text,version`, id, name, data, s.clock.Now().UTC()).Scan(&t.ID, &t.Version)
			})
			return t, templateError(e)
		}, s)
	})
	r.Patch("/api/v1/templates/quests/{id}", func(w http.ResponseWriter, r *http.Request) {
		decode(w, r, func(in TemplateInput) (any, error) {
			id, e := owner(r.Context())
			if e != nil {
				return nil, e
			}
			name, e := templateName(in.Name)
			if e != nil {
				return nil, e
			}
			in.Input, e = Validate(in.Input)
			if e != nil {
				return nil, e
			}
			var t Template
			e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
				if e := s.lockOwner(r.Context(), tx, id); e != nil {
					return e
				}
				old, e := template(r.Context(), tx, id, chi.URLParam(r, "id"))
				if e != nil {
					return e
				}
				if p := platform.MatchVersion(r, old.Version); p != nil {
					return p
				}
				if e := s.labels(r.Context(), tx, id, in.Input); e != nil {
					return e
				}
				t = old
				t.Name = name
				t.Input = reusable(in.Input)
				data, e := json.Marshal(t.Input)
				if e != nil {
					return e
				}
				return tx.QueryRow(r.Context(), `UPDATE quest_templates SET name=$3,payload=$4,version=version+1,updated_at=$5 WHERE owner_id=$1 AND id=$2 RETURNING version`, id, t.ID, name, data, s.clock.Now().UTC()).Scan(&t.Version)
			})
			return t, templateError(e)
		}, s)
	})
	r.Delete("/api/v1/templates/quests/{id}", func(w http.ResponseWriter, r *http.Request) {
		decode(w, r, func(in struct{}) (any, error) {
			id, e := owner(r.Context())
			if e != nil {
				return nil, e
			}
			e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
				if e := s.lockOwner(r.Context(), tx, id); e != nil {
					return e
				}
				t, e := template(r.Context(), tx, id, chi.URLParam(r, "id"))
				if e != nil {
					return e
				}
				if p := platform.MatchVersion(r, t.Version); p != nil {
					return p
				}
				_, e = tx.Exec(r.Context(), `DELETE FROM quest_templates WHERE owner_id=$1 AND id=$2`, id, t.ID)
				return e
			})
			return struct{}{}, e
		}, s)
	})
	for _, action := range []string{"preview", "apply"} {
		r.Post("/api/v1/templates/quests/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			decode(w, r, func(in TemplateDates) (any, error) {
				id, e := owner(r.Context())
				if e != nil {
					return nil, e
				}
				key := chi.URLParam(r, "id")
				if action == "apply" {
					return s.idempotent(r, in, func(tx pgx.Tx) (any, error) {
						if e := s.lockOwner(r.Context(), tx, id); e != nil {
							return nil, e
						}
						preview, e := s.templatePreview(r.Context(), tx, id, key, in)
						if e != nil {
							return nil, e
						}
						if p := platform.MatchVersion(r, preview.Version); p != nil {
							return nil, p
						}
						return s.insert(r.Context(), tx, id, preview.Input)
					})
				}
				var preview TemplatePreview
				e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
					var e error
					preview, e = s.templatePreview(r.Context(), tx, id, key, in)
					return e
				})
				return preview, e
			}, s)
		})
	}
}
