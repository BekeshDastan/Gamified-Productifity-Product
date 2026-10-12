package goals

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"unicode"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type MilestoneInput struct {
	Title     string  `json:"title"`
	Required  bool    `json:"required"`
	Weight    float64 `json:"weight"`
	Completed bool    `json:"completed"`
	Reason    string  `json:"reason"`
}
type Milestone struct {
	MilestoneInput
	ID      string `json:"id"`
	Version int64  `json:"version"`
}
type LinkInput struct {
	MilestoneID         *string `json:"milestone_id"`
	Required            bool    `json:"required"`
	Weight              float64 `json:"weight"`
	PreviousGoalID      *string `json:"previous_goal_id"`
	PreviousGoalVersion int64   `json:"previous_goal_version"`
	Reason              string  `json:"reason"`
}
type Link struct {
	QuestID     string  `json:"quest_id"`
	Title       string  `json:"title"`
	Status      string  `json:"status"`
	MilestoneID *string `json:"milestone_id"`
	Required    bool    `json:"required"`
	Weight      float64 `json:"weight"`
}
type Plan struct {
	Milestones []Milestone `json:"milestones"`
	Links      []Link      `json:"links"`
}
type Assignment struct {
	GoalID      *string `json:"goal_id"`
	GoalTitle   string  `json:"goal_title"`
	GoalVersion int64   `json:"goal_version"`
}

func (s *Service) plan(ctx context.Context, tx interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, id, key string) (Plan, error) {
	out := Plan{Milestones: []Milestone{}, Links: []Link{}}
	rows, e := tx.Query(ctx, `SELECT id::text,title,required,weight::float8,completed,version FROM milestones WHERE owner_id=$1 AND goal_id=$2 ORDER BY created_at,id`, id, key)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var m Milestone
		if e = rows.Scan(&m.ID, &m.Title, &m.Required, &m.Weight, &m.Completed, &m.Version); e != nil {
			rows.Close()
			return out, e
		}
		out.Milestones = append(out.Milestones, m)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.Query(ctx, `SELECT l.quest_id::text,q.payload->>'title',q.status,l.milestone_id::text,l.required,l.weight::float8 FROM goal_quests l JOIN quests q ON q.owner_id=l.owner_id AND q.id=l.quest_id WHERE l.owner_id=$1 AND l.goal_id=$2 ORDER BY q.created_at,q.id`, id, key)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var l Link
		if e = rows.Scan(&l.QuestID, &l.Title, &l.Status, &l.MilestoneID, &l.Required, &l.Weight); e != nil {
			return out, e
		}
		out.Links = append(out.Links, l)
	}
	return out, rows.Err()
}
func (s *Service) editable(r *http.Request, tx pgx.Tx, id, key string) (Goal, error) {
	if e := s.lock(r.Context(), tx, id); e != nil {
		return Goal{}, e
	}
	g, e := s.get(r.Context(), tx, id, key)
	if e != nil {
		return g, e
	}
	if p := platform.MatchVersion(r, g.Version); p != nil {
		return g, p
	}
	if g.ArchivedAt != nil || !strings.Contains("|draft|active|paused|", "|"+g.State+"|") {
		return g, problem(409, "INVALID_TRANSITION", "Сначала откройте цель для пересмотра")
	}
	return g, nil
}

// A linked container expands to mandatory leaves; its own completion is never counted.
const leavesSQL = `WITH RECURSIVE tree AS (
 SELECT l.goal_id,l.quest_id id,l.milestone_id,l.required AND COALESCE((q.payload->>'required')::boolean,true) mandatory,l.weight FROM goal_quests l JOIN quests q ON q.owner_id=l.owner_id AND q.id=l.quest_id WHERE l.owner_id=$1 AND l.goal_id=$2
 UNION ALL SELECT t.goal_id,c.id,t.milestone_id,t.mandatory AND COALESCE((c.payload->>'required')::boolean,true),t.weight FROM tree t JOIN quests c ON c.parent_id=t.id AND c.owner_id=$1
 ), leaves AS (
 SELECT DISTINCT ON(t.id) t.*,q.status FROM tree t JOIN quests q ON q.id=t.id AND q.owner_id=$1
 WHERE NOT EXISTS(SELECT 1 FROM quests c WHERE c.parent_id=t.id AND c.owner_id=$1) ORDER BY t.id
 ) `

func (s *Service) calculate(ctx context.Context, tx pgx.Tx, id string, g Goal) (float64, error) {
	if g.ProgressType == "metric" {
		progress, _, e := s.metricProgress(ctx, tx, id, g)
		return progress, e
	}
	var progress float64
	sql := leavesSQL + `SELECT COALESCE(100*sum(l.weight*COALESCE(m.weight,1)) FILTER(WHERE l.status='completed')/NULLIF(sum(l.weight*COALESCE(m.weight,1)),0),0)::float8 FROM leaves l LEFT JOIN milestones m ON m.owner_id=$1 AND m.goal_id=$2 AND m.id=l.milestone_id WHERE l.mandatory AND COALESCE(m.required,true)`
	if g.ProgressType == "milestones" {
		sql = leavesSQL + `, parts AS (
 SELECT m.weight,CASE WHEN EXISTS(SELECT 1 FROM goal_quests q WHERE q.owner_id=$1 AND q.goal_id=$2 AND q.milestone_id=m.id)
 THEN COALESCE((SELECT sum(l.weight) FILTER(WHERE status='completed')/NULLIF(sum(l.weight),0) FROM leaves l WHERE l.milestone_id=m.id AND l.mandatory),0)
 ELSE CASE WHEN m.completed THEN 1 ELSE 0 END END ratio
 FROM milestones m WHERE m.owner_id=$1 AND m.goal_id=$2 AND m.required)
 SELECT COALESCE(100*sum(weight*ratio)/NULLIF(sum(weight),0),0)::float8 FROM parts`
	}
	e := tx.QueryRow(ctx, sql, id, g.ID).Scan(&progress)
	return progress, e
}
func (s *Service) touch(ctx context.Context, tx pgx.Tx, id string, g Goal, kind, reason string) (Goal, error) {
	progress, e := s.calculate(ctx, tx, id, g)
	if e != nil {
		return g, e
	}
	state := g.State
	if state == "achieved" && progress < 100 {
		state = "active"
		kind = "goal.achievement_reverted"
		reason = "Исправление источников снизило подтверждённый результат. " + reason
	}
	current := float64(0)
	if g.ProgressType == "metric" {
		_, current, e = s.metricProgress(ctx, tx, id, g)
		if e != nil {
			return g, e
		}
	}
	_, e = tx.Exec(ctx, `UPDATE goals SET progress=$3,state=$4,version=version+1,updated_at=$5,current=$6 WHERE owner_id=$1 AND id=$2`, id, g.ID, progress, state, s.clock.Now().UTC(), current)
	if e != nil {
		return g, e
	}
	g, e = s.get(ctx, tx, id, g.ID)
	if e == nil {
		e = s.event(ctx, tx, id, kind, reason, g)
	}
	return g, e
}
func reason(v string) error {
	if !text(v, 1000) {
		return invalid("reason", "До 1000 символов")
	}
	return nil
}
func (s *Service) mountPlan(r chi.Router) {
	r.Get("/api/v1/goals/{id}/plan", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		key := chi.URLParam(r, "id")
		if _, e = s.get(r.Context(), s.pool, id, key); e != nil {
			s.reply(w, r, nil, e)
			return
		}
		v, e := s.plan(r.Context(), s.pool, id, key)
		s.reply(w, r, v, e)
	})
	r.Get("/api/v1/quests/{id}/goal", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		key := chi.URLParam(r, "id")
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		if !uuid(key) {
			s.reply(w, r, nil, problem(404, "NOT_FOUND", "Задача не найдена"))
			return
		}
		var exists bool
		e = s.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM quests WHERE owner_id=$1 AND id=$2)`, id, key).Scan(&exists)
		if e != nil || !exists {
			s.reply(w, r, nil, problem(404, "NOT_FOUND", "Задача не найдена"))
			return
		}
		var out Assignment
		e = s.pool.QueryRow(r.Context(), `SELECT (SELECT g.id::text FROM goal_quests l JOIN goals g ON g.owner_id=l.owner_id AND g.id=l.goal_id WHERE l.owner_id=$1 AND l.quest_id=$2),COALESCE((SELECT g.payload->>'title' FROM goal_quests l JOIN goals g ON g.owner_id=l.owner_id AND g.id=l.goal_id WHERE l.owner_id=$1 AND l.quest_id=$2),''),COALESCE((SELECT g.version FROM goal_quests l JOIN goals g ON g.owner_id=l.owner_id AND g.id=l.goal_id WHERE l.owner_id=$1 AND l.quest_id=$2),0)`, id, key).Scan(&out.GoalID, &out.GoalTitle, &out.GoalVersion)
		s.reply(w, r, out, e)
	})
	for _, method := range []string{"POST", "PATCH", "DELETE"} {
		path := "/api/v1/goals/{id}/milestones"
		if method != "POST" {
			path += "/{mid}"
		}
		r.MethodFunc(method, path, func(w http.ResponseWriter, r *http.Request) {
			decode(s, w, r, func(in MilestoneInput) (any, error) {
				id, e := owner(r.Context())
				if e != nil {
					return nil, e
				}
				in.Title = strings.TrimSpace(in.Title)
				if e := reason(in.Reason); e != nil {
					return nil, e
				}
				if method != "DELETE" && (in.Title == "" || !text(in.Title, 200) || strings.IndexFunc(in.Title, unicode.IsControl) >= 0 || !amount(in.Weight) || in.Weight <= 0 || in.Weight > 1000) {
					return nil, invalid("title", "Название 1–200, вес больше 0 и до 1000")
				}
				var g Goal
				e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
					g, e = s.editable(r, tx, id, chi.URLParam(r, "id"))
					if e != nil {
						return e
					}
					if method == "POST" {
						var count int
						if e = tx.QueryRow(r.Context(), `SELECT count(*) FROM milestones WHERE owner_id=$1 AND goal_id=$2`, id, g.ID).Scan(&count); e != nil {
							return e
						}
						if count >= 100 {
							return invalid("title", "Не более 100 этапов")
						}
						_, e = tx.Exec(r.Context(), `INSERT INTO milestones(owner_id,goal_id,title,required,weight,completed,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, g.ID, in.Title, in.Required, in.Weight, in.Completed, s.clock.Now().UTC())
					} else {
						mid := chi.URLParam(r, "mid")
						if !uuid(mid) {
							return problem(404, "NOT_FOUND", "Этап не найден")
						}
						var found bool
						if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM milestones WHERE owner_id=$1 AND goal_id=$2 AND id=$3)`, id, g.ID, mid).Scan(&found); e != nil {
							return e
						}
						if !found {
							return problem(404, "NOT_FOUND", "Этап не найден")
						}
						if method == "DELETE" {
							if strings.TrimSpace(in.Reason) == "" {
								return invalid("reason", "Укажите причину удаления этапа")
							}
							if _, e = tx.Exec(r.Context(), `UPDATE goal_quests SET milestone_id=NULL WHERE owner_id=$1 AND goal_id=$2 AND milestone_id=$3`, id, g.ID, mid); e == nil {
								_, e = tx.Exec(r.Context(), `DELETE FROM milestones WHERE owner_id=$1 AND goal_id=$2 AND id=$3`, id, g.ID, mid)
							}
						} else {
							_, e = tx.Exec(r.Context(), `UPDATE milestones SET title=$4,required=$5,weight=$6,completed=$7,version=version+1 WHERE owner_id=$1 AND goal_id=$2 AND id=$3`, id, g.ID, mid, in.Title, in.Required, in.Weight, in.Completed)
						}
					}
					if e == nil {
						g, e = s.touch(r.Context(), tx, id, g, "milestone_changed", in.Reason)
					}
					return e
				})
				return g, e
			})
		})
	}
	for _, method := range []string{"PUT", "DELETE"} {
		r.MethodFunc(method, "/api/v1/goals/{id}/links/{qid}", func(w http.ResponseWriter, r *http.Request) {
			decode(s, w, r, func(in LinkInput) (any, error) {
				id, e := owner(r.Context())
				if e != nil {
					return nil, e
				}
				if e = reason(in.Reason); e != nil {
					return nil, e
				}
				qid := chi.URLParam(r, "qid")
				if !uuid(qid) {
					return nil, problem(404, "NOT_FOUND", "Задача не найдена")
				}
				if method == "PUT" && (!amount(in.Weight) || in.Weight <= 0 || in.Weight > 1000 || in.MilestoneID != nil && !uuid(*in.MilestoneID)) {
					return nil, invalid("weight", "Вес больше 0 и до 1000")
				}
				var g Goal
				e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
					g, e = s.editable(r, tx, id, chi.URLParam(r, "id"))
					if e != nil {
						return e
					}
					var exists bool
					if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM quests WHERE owner_id=$1 AND id=$2 AND archived_at IS NULL)`, id, qid).Scan(&exists); e != nil {
						return e
					}
					if !exists {
						return problem(404, "NOT_FOUND", "Задача не найдена")
					}
					var old *string
					if e = tx.QueryRow(r.Context(), `SELECT (SELECT goal_id::text FROM goal_quests WHERE owner_id=$1 AND quest_id=$2)`, id, qid).Scan(&old); e != nil {
						return e
					}
					if method == "DELETE" {
						if old == nil || *old != g.ID {
							return problem(404, "NOT_FOUND", "Связь не найдена")
						}
						if strings.TrimSpace(in.Reason) == "" {
							return invalid("reason", "Укажите причину снятия связи")
						}
						_, e = tx.Exec(r.Context(), `DELETE FROM goal_quests WHERE owner_id=$1 AND quest_id=$2`, id, qid)
					} else {
						if in.MilestoneID != nil {
							if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM milestones WHERE owner_id=$1 AND goal_id=$2 AND id=$3)`, id, g.ID, *in.MilestoneID).Scan(&exists); e != nil {
								return e
							}
							if !exists {
								return invalid("milestone_id", "Этап не найден")
							}
						}
						if old != nil && *old != g.ID {
							previous, e := s.get(r.Context(), tx, id, *old)
							if e != nil {
								return e
							}
							if in.PreviousGoalID == nil || *in.PreviousGoalID != previous.ID || in.PreviousGoalVersion != previous.Version || strings.TrimSpace(in.Reason) == "" {
								return problem(409, "GOAL_TRANSFER_CONFIRMATION", "Подтвердите перенос из актуальной версии прежней цели")
							}
							if previous.ArchivedAt != nil || !strings.Contains("|draft|active|paused|", "|"+previous.State+"|") {
								return problem(409, "INVALID_TRANSITION", "Сначала пересмотрите прежнюю цель")
							}
						}
						// Overlapping roots would count the same leaf in multiple plans.
						if e = tx.QueryRow(r.Context(), `WITH RECURSIVE relatives AS (SELECT id,parent_id FROM quests WHERE owner_id=$1 AND id=$2 UNION SELECT q.id,q.parent_id FROM quests q JOIN relatives t ON q.id=t.parent_id WHERE q.owner_id=$1), descendants AS (SELECT id FROM quests WHERE owner_id=$1 AND id=$2 UNION SELECT q.id FROM quests q JOIN descendants t ON q.parent_id=t.id WHERE q.owner_id=$1) SELECT EXISTS(SELECT 1 FROM goal_quests WHERE owner_id=$1 AND quest_id<>$2 AND quest_id IN(SELECT id FROM relatives UNION SELECT id FROM descendants))`, id, qid).Scan(&exists); e != nil {
							return e
						}
						if exists {
							return invalid("quest_id", "Связь с родителем или потомком уже существует")
						}
						_, e = tx.Exec(r.Context(), `INSERT INTO goal_quests(owner_id,goal_id,quest_id,milestone_id,required,weight) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(quest_id) DO UPDATE SET goal_id=excluded.goal_id,milestone_id=excluded.milestone_id,required=excluded.required,weight=excluded.weight`, id, g.ID, qid, in.MilestoneID, in.Required, in.Weight)
						if e == nil && old != nil && *old != g.ID {
							previous, err := s.get(r.Context(), tx, id, *old)
							if err != nil {
								return err
							}
							_, e = s.touch(r.Context(), tx, id, previous, "quest_transferred", in.Reason)
						}
					}
					if e == nil {
						g, e = s.touch(r.Context(), tx, id, g, "links_changed", in.Reason)
					}
					return e
				})
				return g, e
			})
		})
	}
}

// Plan snapshots stay with the goal version even after a milestone is removed.
func (s *Service) snapshot(ctx context.Context, tx pgx.Tx, id string, g Goal) ([]byte, error) {
	p, e := s.plan(ctx, tx, id, g.ID)
	if e != nil {
		return nil, e
	}
	return json.Marshal(struct {
		Goal
		Plan Plan `json:"plan"`
	}{g, p})
}
