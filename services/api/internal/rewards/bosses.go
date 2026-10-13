package rewards

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type Boss struct {
	ID          string              `json:"id"`
	GoalID      string              `json:"goal_id"`
	Title       string              `json:"title"`
	State       string              `json:"state"`
	Status      string              `json:"status"`
	Version     int64               `json:"version"`
	GoalVersion int64               `json:"goal_version"`
	HP          float64             `json:"hp"`
	Progress    float64             `json:"progress"`
	Stages      []goals.BattleStage `json:"stages"`
	WonAt       *time.Time          `json:"won_at"`
}

func (s *Service) boss(ctx context.Context, tx pgx.Tx, owner, id string) (Boss, error) {
	out := Boss{}
	if !validID(id) {
		return out, problem(404, "NOT_FOUND", "Босс не найден")
	}
	e := tx.QueryRow(ctx, "SELECT id::text,goal_id::text,state,version,won_at FROM bosses WHERE owner_id=$1 AND id=$2", owner, id).Scan(&out.ID, &out.GoalID, &out.State, &out.Version, &out.WonAt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = problem(404, "NOT_FOUND", "Босс не найден")
	}
	if e != nil {
		return out, e
	}
	g, stages, e := goals.NewService(s.pool, s.clock).BattleSourceInTx(ctx, tx, owner, out.GoalID)
	if e != nil {
		return out, e
	}
	out.Title = g.Title
	out.GoalVersion = g.Version
	out.Progress = g.Progress
	out.HP = max(0, min(100, 100-g.Progress))
	out.Stages = stages
	out.Status = out.State
	if g.ArchivedAt != nil {
		out.Status = "goal_archived"
	} else if out.State == "active" {
		if g.State == "achieved" && out.HP == 0 {
			out.Status = "victory"
		} else if out.HP == 0 {
			out.Status = "awaiting_confirmation"
		} else if g.State == "paused" {
			out.Status = "goal_paused"
		}
	}
	var last int64
	if e = tx.QueryRow(ctx, "SELECT last_goal_version FROM bosses WHERE owner_id=$1 AND id=$2", owner, id).Scan(&last); e != nil {
		return out, e
	}
	if last != g.Version {
		if e = s.bossEvent(ctx, tx, owner, out, "Изменился источник цели"); e != nil {
			return out, e
		}
		_, e = tx.Exec(ctx, "UPDATE bosses SET last_goal_version=$3 WHERE owner_id=$1 AND id=$2", owner, id, g.Version)
	}
	return out, e
}
func (s *Service) bossEvent(ctx context.Context, tx pgx.Tx, owner string, b Boss, reason string) error {
	snapshot, e := json.Marshal(b)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, "INSERT INTO boss_events(owner_id,boss_id,snapshot,reason,created_at) VALUES($1,$2,$3,$4,$5)", owner, b.ID, snapshot, reason, s.clock.Now())
	return e
}
func (s *Service) bosses(ctx context.Context, tx pgx.Tx, owner string) ([]Boss, error) {
	rows, e := tx.Query(ctx, "SELECT id::text FROM bosses WHERE owner_id=$1 ORDER BY created_at DESC,id", owner)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	out := []Boss{}
	if e != nil {
		return out, e
	}
	for _, id := range ids {
		b, e := s.boss(ctx, tx, owner, id)
		if e != nil {
			return out, e
		}
		out = append(out, b)
	}
	return out, nil
}
func (s *Service) mountBosses(r chi.Router) {
	r.Get("/api/v1/bosses", func(w http.ResponseWriter, r *http.Request) {
		owner, e := owner(r.Context())
		out := []Boss{}
		if e == nil {
			e = s.progress.WithinOwner(r.Context(), owner, func(tx pgx.Tx) error {
				if e := s.character.EvaluateInTx(r.Context(), tx, owner); e != nil {
					return e
				}
				var e error
				out, e = s.bosses(r.Context(), tx, owner)
				return e
			})
		}
		s.reply(w, r, out, e)
	})
	r.Post("/api/v1/bosses", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in struct {
			GoalID string `json:"goal_id"`
		}) (any, error) { return s.mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			g, stages, e := goals.NewService(s.pool, s.clock).BattleSourceInTx(ctx, tx, owner, in.GoalID)
			if e != nil {
				return nil, e
			}
			if g.ArchivedAt != nil || g.State == "cancelled" || g.State == "not_achieved" {
				return nil, problem(409, "GOAL_UNAVAILABLE", "Цель недоступна для боя")
			}
			required := false
			for _, stage := range stages {
				required = required || stage.Required
			}
			if !required {
				return nil, problem(422, "STAGES_REQUIRED", "Сначала добавьте обязательные этапы или задачи в цель")
			}
			var existing bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM bosses WHERE owner_id=$1 AND goal_id=$2)", owner, g.ID).Scan(&existing); e != nil {
				return nil, e
			}
			if existing {
				return nil, problem(409, "BOSS_EXISTS", "Для этой цели уже есть босс")
			}
			var count int
			if e = tx.QueryRow(ctx, "SELECT count(*) FROM bosses WHERE owner_id=$1", owner).Scan(&count); e != nil {
				return nil, e
			}
			if count >= 100 {
				return nil, problem(409, "LIMIT_REACHED", "Не более 100 боссов")
			}
			var id string
			e = tx.QueryRow(ctx, "INSERT INTO bosses(owner_id,goal_id,state,created_at,updated_at) VALUES($1,$2,'active',$3,$3) RETURNING id::text", owner, g.ID, s.clock.Now()).Scan(&id)
			if e != nil {
				return nil, e
			}
			if e = s.character.EvaluateInTx(ctx, tx, owner); e != nil {
				return nil, e
			}
			return s.boss(ctx, tx, owner, id)
		}) })
	})
	r.Patch("/api/v1/bosses/{id}", func(w http.ResponseWriter, r *http.Request) {
		decode(s, w, r, func(in struct {
			Action string `json:"action"`
			Reason string `json:"reason"`
		}) (any, error) { return s.mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			b, e := s.boss(ctx, tx, owner, chi.URLParam(r, "id"))
			if e != nil {
				return nil, e
			}
			if p := platform.MatchVersion(r, b.Version); p != nil {
				return nil, p
			}
			state := ""
			switch in.Action {
			case "pause":
				if b.State == "active" {
					state = "paused"
				}
			case "resume":
				if b.State != "active" {
					state = "active"
				}
			case "close":
				if b.State != "closed" {
					state = "closed"
				}
			}
			reason := strings.TrimSpace(in.Reason)
			if state == "" {
				return nil, problem(409, "INVALID_TRANSITION", "Действие недоступно")
			}
			if len(reason) == 0 || utf8.RuneCountInString(reason) > 1000 {
				return nil, problem(422, "VALIDATION_ERROR", "Укажите причину до 1000 символов")
			}
			_, e = tx.Exec(ctx, "UPDATE bosses SET state=$3,version=version+1,updated_at=$4 WHERE owner_id=$1 AND id=$2", owner, b.ID, state, s.clock.Now())
			if e != nil {
				return nil, e
			}
			if e = s.character.EvaluateInTx(ctx, tx, owner); e != nil {
				return nil, e
			}
			b, e = s.boss(ctx, tx, owner, b.ID)
			if e != nil {
				return nil, e
			}
			return b, s.bossEvent(ctx, tx, owner, b, reason)
		}) })
	})
}
