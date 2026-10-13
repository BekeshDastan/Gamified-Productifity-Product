package journal

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type ChecklistStep struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type RoutineTemplate struct {
	Kind    string          `json:"kind"`
	Version int64           `json:"version"`
	Steps   []ChecklistStep `json:"steps"`
}
type TemplateInput struct {
	Steps []ChecklistStep `json:"steps"`
}

var stepID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func templateKind(kind string) error {
	if kind != "morning" && kind != "evening" {
		return privateapi.Problem(404, "NOT_FOUND", "Ритуал не найден")
	}
	return nil
}
func defaultTemplate(kind string) RoutineTemplate {
	steps := []ChecklistStep{{"time", "Оценить доступное время"}, {"priorities", "Выбрать главные задачи"}, {"confirm", "Подтвердить план дня"}}
	if kind == "evening" {
		steps = []ChecklistStep{{"outcomes", "Посмотреть реальные итоги"}, {"obstacles", "Отметить препятствия"}, {"lesson", "Записать один вывод"}}
	}
	return RoutineTemplate{Kind: kind, Steps: steps}
}
func (s *Service) template(ctx context.Context, tx pgx.Tx, owner, kind string) (RoutineTemplate, error) {
	out := defaultTemplate(kind)
	if e := templateKind(kind); e != nil {
		return out, e
	}
	var data []byte
	e := tx.QueryRow(ctx, "SELECT steps,version FROM journal_routine_templates WHERE owner_id=$1 AND kind=$2", owner, kind).Scan(&data, &out.Version)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(data, &out.Steps)
	return out, e
}
func validateTemplate(in TemplateInput) (TemplateInput, error) {
	if len(in.Steps) > 20 {
		return in, privateapi.Invalid("До 20 шагов в ритуале")
	}
	if in.Steps == nil {
		in.Steps = []ChecklistStep{}
	}
	seen := map[string]bool{}
	for i, step := range in.Steps {
		step.Label = strings.TrimSpace(step.Label)
		if !stepID.MatchString(step.ID) || seen[step.ID] || step.Label == "" || utf8.RuneCountInString(step.Label) > 200 || strings.IndexFunc(step.Label, unicode.IsControl) >= 0 {
			return in, privateapi.Invalid("Каждый шаг: уникальный ID и название до 200 символов")
		}
		seen[step.ID] = true
		in.Steps[i] = step
	}
	return in, nil
}
func (s *Service) checkTemplate(ctx context.Context, tx pgx.Tx, owner, kind string, version int64, checked []string) (RoutineTemplate, error) {
	t, e := s.template(ctx, tx, owner, kind)
	if e != nil {
		return t, e
	}
	if version != t.Version {
		return t, privateapi.Problem(409, "TEMPLATE_CHANGED", "Чек-лист изменился. Обновите его перед сохранением")
	}
	allowed := map[string]bool{}
	for _, step := range t.Steps {
		allowed[step.ID] = true
	}
	seen := map[string]bool{}
	for _, id := range checked {
		if !allowed[id] || seen[id] {
			return t, privateapi.Invalid("Выберите разные шаги из текущего чек-листа")
		}
		seen[id] = true
	}
	return t, nil
}
func (s *Service) saveTemplate(r *http.Request, tx pgx.Tx, owner, kind string, in TemplateInput) (RoutineTemplate, error) {
	in, e := validateTemplate(in)
	if e != nil {
		return RoutineTemplate{}, e
	}
	old, e := s.template(r.Context(), tx, owner, kind)
	if e != nil {
		return old, e
	}
	if p := privateapi.MatchVersion(r, old.Version); p != nil {
		return old, p
	}
	data, e := json.Marshal(in.Steps)
	if e != nil {
		return old, e
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO journal_routine_templates(owner_id,kind,steps,updated_at) VALUES($1,$2,$3,$4) ON CONFLICT(owner_id,kind) DO UPDATE SET steps=excluded.steps,version=journal_routine_templates.version+1,updated_at=excluded.updated_at`, owner, kind, data, s.Clock.Now().UTC())
	if e != nil {
		return old, e
	}
	return s.template(r.Context(), tx, owner, kind)
}

type RunHistoryItem struct {
	ID       string `json:"id"`
	Snapshot Run    `json:"snapshot"`
}
type RunHistory struct {
	Items []RunHistoryItem `json:"items"`
	Next  string           `json:"next"`
}

func (s *Service) history(r *http.Request, tx pgx.Tx, owner string) (RunHistory, error) {
	out := RunHistory{Items: []RunHistoryItem{}}
	before := int64(1<<63 - 1)
	var e error
	if raw := r.URL.Query().Get("before"); raw != "" {
		before, e = strconv.ParseInt(raw, 10, 64)
		if e != nil || before < 1 {
			return out, privateapi.Invalid("Некорректная страница истории")
		}
	}
	rows, e := tx.Query(r.Context(), "SELECT id::text,snapshot FROM routine_run_events WHERE owner_id=$1 AND id<$2 ORDER BY id DESC LIMIT 51", owner, before)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v RunHistoryItem
		var data []byte
		if e = rows.Scan(&v.ID, &data); e != nil {
			return out, e
		}
		if e = json.Unmarshal(data, &v.Snapshot); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > 50 {
		out.Items = out.Items[:50]
		out.Next = out.Items[49].ID
	}
	return out, nil
}
func (s *Service) mountTemplates(r chi.Router) {
	r.Get("/api/v1/routine-runs/templates/{kind}", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.template(ctx, tx, owner, chi.URLParam(r, "kind"))
		})
		s.Reply(w, r, out, e)
	})
	r.Put("/api/v1/routine-runs/templates/{kind}", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in TemplateInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.saveTemplate(r, tx, owner, chi.URLParam(r, "kind"), in)
			})
		})
	})
	r.Get("/api/v1/routine-runs", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) { return s.history(r, tx, owner) })
		s.Reply(w, r, out, e)
	})
}
