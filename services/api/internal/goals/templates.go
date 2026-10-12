package goals

import (
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type DraftTask struct {
	Title            string `json:"title"`
	Description      string `json:"description"`
	DayOffset        int    `json:"day_offset"`
	Milestone        int    `json:"milestone"`
	Required         bool   `json:"required"`
	EstimatedMinutes int    `json:"estimated_minutes"`
	Difficulty       int    `json:"difficulty"`
}
type PlannedTask struct {
	DraftTask
	PlannedDate string `json:"planned_date"`
}
type GoalTemplate struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Input       Input            `json:"input"`
	Milestones  []MilestoneInput `json:"milestones"`
	Tasks       []DraftTask      `json:"tasks"`
}
type TemplateDraft struct {
	TemplateID   string           `json:"template_id"`
	Input        Input            `json:"input"`
	DurationDays int              `json:"duration_days"`
	Milestones   []MilestoneInput `json:"milestones"`
	Tasks        []DraftTask      `json:"tasks"`
	Confirmed    bool             `json:"confirmed"`
}
type GoalPreview struct {
	Input      Input            `json:"input"`
	Milestones []MilestoneInput `json:"milestones"`
	Tasks      []PlannedTask    `json:"tasks"`
	Notice     string           `json:"notice"`
}

func catalog() []GoalTemplate {
	specs := []struct {
		id, name, why, kind, unit string
		target                    float64
		parts                     []string
	}{
		{"english-practice", "Английская практика", "Выберите удобную нагрузку. Количество минут не гарантирует уровень языка.", "metric", "минуты", 900, []string{"Определить темы", "Практиковаться", "Подвести итоги"}},
		{"exam", "Подготовка к экзамену", "Составьте собственную программу. План не гарантирует оценку.", "milestones", "", 1, []string{"Разобрать программу", "Закрепить материал", "Решить пробный вариант"}},
		{"internship", "Поиск стажировки", "Адаптируйте план под свою ситуацию. Он не гарантирует трудоустройство.", "quests", "", 1, []string{"Обновить резюме", "Подготовить портфолио", "Отправить выбранные отклики"}},
		{"personal-project", "Личный проект", "Определите небольшой проверяемый результат.", "milestones", "", 1, []string{"Описать результат", "Сделать первую версию", "Проверить и опубликовать"}},
		{"reading", "Чтение книг", "Выберите книги и собственный темп. Результаты вносите по факту.", "metric", "книги", 12, []string{"Выбрать книги", "Запланировать чтение", "Записать выводы"}},
		{"home-order", "Порядок дома", "Выберите небольшой объём и комфортный темп.", "quests", "", 1, []string{"Выбрать одну зону", "Разобрать вещи", "Закрепить порядок"}},
	}
	out := []GoalTemplate{}
	for _, v := range specs {
		t := GoalTemplate{ID: v.id, Name: v.name, Description: v.why, Input: Input{Title: v.name, Motive: v.why, Criteria: "Выполнить согласованный план и проверить результат", ProgressType: v.kind, Unit: v.unit, Target: v.target}, Milestones: []MilestoneInput{}, Tasks: []DraftTask{}}
		if v.kind == "metric" {
			t.Input.Criteria = "Достичь выбранного количества и подтвердить результат"
		}
		for i, title := range v.parts {
			t.Milestones = append(t.Milestones, MilestoneInput{Title: title, Weight: 1, Required: true})
			t.Tasks = append(t.Tasks, DraftTask{Title: title, DayOffset: i * 10, Milestone: i, Required: true, EstimatedMinutes: 30, Difficulty: 2})
		}
		out = append(out, t)
	}
	return out
}
func preview(in TemplateDraft) (GoalPreview, error) {
	found := false
	for _, t := range catalog() {
		if t.ID == in.TemplateID {
			found = true
		}
	}
	if !found {
		return GoalPreview{}, invalid("template_id", "Шаблон не найден")
	}
	if in.Input.StartDate == nil || !date(in.Input.StartDate) || in.Input.TargetDate != nil || in.DurationDays < 1 || in.DurationDays > 3660 {
		return GoalPreview{}, invalid("duration_days", "Укажите начальную дату и длительность 1–3660 дней")
	}
	start, _ := time.Parse("2006-01-02", *in.Input.StartDate)
	end := start.AddDate(0, 0, in.DurationDays-1).Format("2006-01-02")
	in.Input.TargetDate = &end
	input, e := Validate(in.Input)
	if e != nil {
		return GoalPreview{}, e
	}
	if len(in.Milestones) > 20 || len(in.Tasks) > 100 {
		return GoalPreview{}, invalid("tasks", "Не более 20 этапов и 100 задач в шаблоне")
	}
	out := GoalPreview{Input: input, Milestones: in.Milestones, Tasks: []PlannedTask{}, Notice: "Проверьте даты и нагрузку. Этот план не гарантирует уровень знаний, оценку или трудоустройство."}
	for i, m := range in.Milestones {
		m.Title = strings.TrimSpace(m.Title)
		if m.Title == "" || !text(m.Title, 200) || strings.IndexFunc(m.Title, unicode.IsControl) >= 0 || !amount(m.Weight) || m.Weight <= 0 || m.Weight > 1000 || m.Completed {
			return out, invalid("milestones", "Новые этапы имеют название, положительный вес и не выполнены")
		}
		m.Reason = ""
		out.Milestones[i] = m
	}
	for _, t := range in.Tasks {
		t.Title = strings.TrimSpace(t.Title)
		if t.DayOffset < 0 || t.DayOffset >= in.DurationDays || t.Milestone < -1 || t.Milestone >= len(in.Milestones) {
			return out, invalid("tasks", "Проверьте день задачи и номер её этапа")
		}
		if _, e = quests.Validate(quests.Input{Title: t.Title, Description: t.Description, Priority: "normal", Difficulty: t.Difficulty, EstimatedMinutes: t.EstimatedMinutes, Tags: []string{}, Status: "todo", Required: t.Required}); e != nil {
			return out, invalid("tasks", "Проверьте название, сложность и оценку задачи")
		}
		out.Tasks = append(out.Tasks, PlannedTask{DraftTask: t, PlannedDate: start.AddDate(0, 0, t.DayOffset).Format("2006-01-02")})
	}
	return out, nil
}
func (s *Service) mountTemplates(r chi.Router) {
	r.Get("/api/v1/templates/goals", func(w http.ResponseWriter, r *http.Request) { s.reply(w, r, catalog(), nil) })
	for _, apply := range []bool{false, true} {
		path := "/api/v1/templates/goals/preview"
		if apply {
			path = "/api/v1/templates/goals/apply"
		}
		r.Post(path, func(w http.ResponseWriter, r *http.Request) {
			decode(s, w, r, func(in TemplateDraft) (any, error) {
				id, e := owner(r.Context())
				if e != nil {
					return nil, e
				}
				p, e := preview(in)
				if e != nil {
					return nil, e
				}
				if !apply {
					if p.Input.AreaID != nil {
						var exists bool
						e = s.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM life_areas WHERE owner_id=$1 AND id=$2)`, id, *p.Input.AreaID).Scan(&exists)
						if e != nil {
							return nil, e
						}
						if !exists {
							return nil, invalid("area_id", "Направление не найдено")
						}
					}
					return p, nil
				}
				if !in.Confirmed {
					return nil, invalid("confirmed", "Подтвердите проверенный план")
				}
				return s.idempotent(r, in, func(tx pgx.Tx) (any, error) {
					if e := s.lock(r.Context(), tx, id); e != nil {
						return nil, e
					}
					g, e := s.insert(r.Context(), tx, id, Create{Input: p.Input, State: "draft"})
					if e != nil {
						return nil, e
					}
					ids := []string{}
					for _, m := range p.Milestones {
						var mid string
						e = tx.QueryRow(r.Context(), `INSERT INTO milestones(owner_id,goal_id,title,required,weight,completed,created_at) VALUES($1,$2,$3,$4,$5,false,$6) RETURNING id::text`, id, g.ID, m.Title, m.Required, m.Weight, s.clock.Now().UTC()).Scan(&mid)
						if e != nil {
							return nil, e
						}
						ids = append(ids, mid)
					}
					for _, task := range p.Tasks {
						q, e := quests.NewService(s.pool, s.clock).CreateInTx(r.Context(), tx, id, quests.Input{Title: task.Title, Description: task.Description, AreaID: p.Input.AreaID, Priority: "normal", Difficulty: task.Difficulty, PlannedDate: &task.PlannedDate, EstimatedMinutes: task.EstimatedMinutes, Tags: []string{}, Status: "todo", Required: task.Required})
						if e != nil {
							return nil, e
						}
						var mid *string
						if task.Milestone >= 0 {
							mid = &ids[task.Milestone]
						}
						if _, e = tx.Exec(r.Context(), `INSERT INTO goal_quests(owner_id,goal_id,quest_id,milestone_id,required,weight) VALUES($1,$2,$3,$4,$5,1)`, id, g.ID, q.ID, mid, task.Required); e != nil {
							return nil, e
						}
					}
					return s.touch(r.Context(), tx, id, g, "template_applied", "Подтверждённый план: "+in.TemplateID)
				})
			})
		})
	}
}
