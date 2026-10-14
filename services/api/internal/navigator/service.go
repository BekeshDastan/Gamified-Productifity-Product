package navigator

import (
	"context"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"
)

type Service struct {
	*privateapi.Service
	plans     *planning.Service
	goals     *goals.Service
	habits    *habits.Service
	analytics *analytics.Service
	quests    *quests.Service
}

func NewService(pool *pgxpool.Pool, clock platform.Clock, p *planning.Service, g *goals.Service, h *habits.Service, a *analytics.Service) *Service {
	return &Service{Service: privateapi.New(pool, clock), plans: p, goals: g, habits: h, analytics: a}
}

type Setting struct {
	RuleID   string `json:"rule_id"`
	Disabled bool   `json:"disabled"`
	Version  int64  `json:"version"`
}
type Card struct {
	Candidate
	ID        string    `json:"id"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Message   string    `json:"message"`
	ActionURL string    `json:"action_url"`
}
type Page struct {
	Items      []Card    `json:"items"`
	Settings   []Setting `json:"settings"`
	Rules      []Rule    `json:"rules"`
	ComputedAt time.Time `json:"computed_at"`
	Complete   bool      `json:"complete"`
}

func (s *Service) rules(ctx context.Context, tx pgx.Tx) ([]Rule, error) {
	rows, e := tx.Query(ctx, `SELECT DISTINCT ON(id) payload FROM navigator_rule_versions ORDER BY id,version DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		var raw []byte
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		r, e := DecodeRule(raw)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Service) page(ctx context.Context, tx pgx.Tx, owner string) (Page, error) {
	out := Page{Items: []Card{}, Settings: []Setting{}, ComputedAt: s.Clock.Now()}
	var e error
	out.Rules, e = s.rules(ctx, tx)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT rule_id,disabled,version FROM navigator_rule_preferences WHERE owner_id=$1 ORDER BY rule_id`, owner)
	if e != nil {
		return out, e
	}
	disabled := map[string]bool{}
	for rows.Next() {
		var v Setting
		if e = rows.Scan(&v.RuleID, &v.Disabled, &v.Version); e != nil {
			rows.Close()
			return out, e
		}
		out.Settings = append(out.Settings, v)
		disabled[v.RuleID] = v.Disabled
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	facts, complete, e := s.facts(ctx, tx, owner)
	if e != nil {
		return out, e
	}
	out.Complete = complete
	candidates, e := Evaluate(out.Rules, facts)
	if e != nil {
		return out, e
	}
	for _, c := range candidates {
		if disabled[c.Rule.ID] {
			continue
		}
		var id string
		var version int64
		var created time.Time
		var hidden bool
		var until *time.Time
		e = tx.QueryRow(ctx, `SELECT id::text,version,created_at,hidden,snoozed_until FROM navigator_cards WHERE owner_id=$1 AND rule_id=$2 AND rule_version=$3 AND subject=$4 ORDER BY created_at DESC,id DESC LIMIT 1`, owner, c.Rule.ID, c.Rule.Version, c.Fact.Subject).Scan(&id, &version, &created, &hidden, &until)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return out, e
		}
		if e == nil && out.ComputedAt.Before(created.Add(time.Duration(c.Rule.CooldownSeconds)*time.Second)) {
			if hidden || (until != nil && out.ComputedAt.Before(*until)) {
				continue
			}
		} else {
			e = tx.QueryRow(ctx, `INSERT INTO navigator_cards(owner_id,rule_id,rule_version,subject,created_at) VALUES($1,$2,$3,$4,$5) RETURNING id::text,version,created_at`, owner, c.Rule.ID, c.Rule.Version, c.Fact.Subject, out.ComputedAt).Scan(&id, &version, &created)
			if e != nil {
				return out, e
			}
		}
		out.Items = append(out.Items, Card{c, id, version, created, messages[c.Rule.ID], actionURL(c)})
		if len(out.Items) == 3 {
			break
		}
	}
	return out, nil
}

var messages = map[string]string{"R01": "Задачу переносили не менее трёх раз за 14 дней. Можно выделить первый шаг или пересмотреть её актуальность.", "R02": "Оценка выбранных задач превышает доступные минуты. Просмотрите будущий план.", "R03": "До срока меньше суток; оценка превышает свободные рабочие окна.", "R04": "В двух последних закрытых обязательных периодах норма не достигнута. Можно пересмотреть будущую норму.", "R05": "После трёх дней без подтверждённых действий доступен план возвращения.", "R06": "Вы выбрали низкую энергию. Можно просмотреть минимальный план; срочные обязательства сохраняются.", "R07": "Задача была открыта более часа активного просмотра без первого шага. Можно начать с пяти минут. Время просмотра не оценивает ваши усилия.", "R08": "Завершилась неделя, её обзор ещё не сохранён.", "R09": "В выборке шаблона не менее пяти завершений; фактическое время превышает оценку более чем на 50%.", "R10": "У активной цели нет следующего действия. Можно выбрать и отредактировать первый шаг.", "R11": "Привычка выполнена семь запланированных периодов подряд. Посмотрите достижения.", "R12": "Календарные обязательства пересекаются. Выберите перенос или осознанно оставьте конфликт.", "R13": "Срок цели прошёл, при этом уже есть прогресс. Можно пересмотреть срок или подвести итог.", "R14": "В этом месяце ещё не выбран Boss of the Month. Можно выбрать существующую цель.", "R15": "Есть итоги фокуса, ожидающие вашего подтверждения."}

func actionURL(c Candidate) string {
	switch c.Rule.ID {
	case "R01", "R07", "R04", "R10", "R13":
		if len(c.Fact.Sources) > 0 {
			return c.Fact.Sources[0].URL
		}
	case "R02", "R06":
		return "/app/calendar"
	case "R03":
		return "/app/navigator#deadline-radar"
	case "R05":
		return "/app/character"
	case "R08":
		return "/app/journal"
	case "R09":
		return "/app/analytics"
	case "R11":
		return "/app/character"
	case "R12":
		return "/app/calendar"
	case "R14":
		return "/app/rewards"
	case "R15":
		return "/app/focus"
	}
	return "/app/navigator"
}
func (s *Service) Mount(router chi.Router, protect func(http.Handler) http.Handler) {
	router.Group(func(r chi.Router) {
		r.Use(protect)
		s.mountGoalAction(r)
		r.Get("/api/v1/navigator", func(w http.ResponseWriter, r *http.Request) {
			out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) { return s.page(ctx, tx, owner) })
			s.Reply(w, r, out, e)
		})
		r.Put("/api/v1/navigator/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
			privateapi.Decode(s.Service, w, r, func(in Setting) (any, error) {
				return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
					id := chi.URLParam(r, "id")
					if _, ok := metrics[id]; !ok || in.RuleID != id {
						return nil, privateapi.Invalid("Выберите правило R01–R15")
					}
					var current int64
					e := tx.QueryRow(ctx, `SELECT version FROM navigator_rule_preferences WHERE owner_id=$1 AND rule_id=$2`, owner, id).Scan(&current)
					if e != nil && !errors.Is(e, pgx.ErrNoRows) {
						return nil, e
					}
					if e := privateapi.MatchVersion(r, current); e != nil {
						return nil, e
					}
					in.Version = current + 1
					_, e = tx.Exec(ctx, `INSERT INTO navigator_rule_preferences(owner_id,rule_id,disabled,version) VALUES($1,$2,$3,$4) ON CONFLICT(owner_id,rule_id) DO UPDATE SET disabled=excluded.disabled,version=excluded.version`, owner, id, in.Disabled, in.Version)
					return in, e
				})
			})
		})
		r.Post("/api/v1/navigator/cards/{id}/decision", func(w http.ResponseWriter, r *http.Request) {
			privateapi.Decode(s.Service, w, r, func(in struct {
				Action string `json:"action"`
				Hours  int    `json:"hours"`
			}) (any, error) {
				return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
					id := chi.URLParam(r, "id")
					if !privateapi.UUID(id) {
						return nil, privateapi.Problem(404, "NOT_FOUND", "Подсказка не найдена")
					}
					var version int64
					e := tx.QueryRow(ctx, `SELECT version FROM navigator_cards WHERE owner_id=$1 AND id=$2`, owner, id).Scan(&version)
					if errors.Is(e, pgx.ErrNoRows) {
						return nil, privateapi.Problem(404, "NOT_FOUND", "Подсказка не найдена")
					}
					if e != nil {
						return nil, e
					}
					if e := privateapi.MatchVersion(r, version); e != nil {
						return nil, e
					}
					if in.Action != "hide" && in.Action != "snooze" {
						return nil, privateapi.Invalid("Выберите скрытие или отсрочку")
					}
					var until *time.Time
					if in.Action == "snooze" {
						if in.Hours != 24 && in.Hours != 168 {
							return nil, privateapi.Invalid("Отложить на день или неделю")
						}
						at := s.Clock.Now().Add(time.Duration(in.Hours) * time.Hour)
						until = &at
					}
					_, e = tx.Exec(ctx, `UPDATE navigator_cards SET hidden=$3,snoozed_until=$4,version=version+1 WHERE owner_id=$1 AND id=$2`, owner, id, in.Action == "hide", until)
					return map[string]any{"id": id, "version": version + 1}, e
				})
			})
		})
		r.Post("/api/v1/navigator/task-view/{id}", func(w http.ResponseWriter, r *http.Request) {
			privateapi.Decode(s.Service, w, r, func(in struct {
				Visible bool `json:"visible"`
			}) (any, error) {
				return s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
					id := chi.URLParam(r, "id")
					if !privateapi.UUID(id) {
						return nil, privateapi.Invalid("Выберите задачу")
					}
					var own bool
					e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quests WHERE owner_id=$1 AND id=$2 AND archived_at IS NULL AND status NOT IN('completed','failed','skipped'))`, owner, id).Scan(&own)
					if e != nil {
						return nil, e
					}
					if !own {
						return nil, privateapi.Problem(404, "NOT_FOUND", "Задача недоступна")
					}
					_, e = tx.Exec(ctx, `INSERT INTO navigator_task_views(owner_id,quest_id,last_seen,visible) VALUES($1,$2,$3,$4) ON CONFLICT(owner_id,quest_id) DO UPDATE SET visible_seconds=navigator_task_views.visible_seconds+CASE WHEN navigator_task_views.visible AND excluded.visible AND excluded.last_seen-navigator_task_views.last_seen BETWEEN interval '0 seconds' AND interval '45 seconds' THEN LEAST(30,EXTRACT(EPOCH FROM excluded.last_seen-navigator_task_views.last_seen)::integer) ELSE 0 END,last_seen=excluded.last_seen,visible=excluded.visible`, owner, id, s.Clock.Now(), in.Visible)
					return map[string]bool{"accepted": e == nil}, e
				})
			})
		})
	})
}
