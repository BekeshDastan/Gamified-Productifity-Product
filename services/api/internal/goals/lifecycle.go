package goals

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

type Transition struct {
	Reason            string `json:"reason"`
	CriteriaConfirmed bool   `json:"criteria_confirmed"`
	Completed         string `json:"completed"`
	Obstacles         string `json:"obstacles"`
	Resolution        string `json:"resolution"`
	NextInput         *Input `json:"next_input"`
}
type Version struct {
	ID          int64           `json:"id"`
	GoalVersion int64           `json:"goal_version"`
	Kind        string          `json:"kind"`
	Reason      string          `json:"reason"`
	Snapshot    json.RawMessage `json:"snapshot"`
	CreatedAt   time.Time       `json:"created_at"`
}

func closed(state string) bool {
	return state == "achieved" || state == "not_achieved" || state == "cancelled"
}
func (s *Service) mountLifecycle(r chi.Router) {
	for _, action := range []string{"pause", "resume", "achieve", "close-unachieved", "cancel", "review", "archive", "restore"} {
		r.Post("/api/v1/goals/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			decode(s, w, r, func(in Transition) (any, error) {
				id, e := owner(r.Context())
				if e != nil {
					return nil, e
				}
				in.Reason = strings.TrimSpace(in.Reason)
				in.Completed = strings.TrimSpace(in.Completed)
				in.Obstacles = strings.TrimSpace(in.Obstacles)
				if e = reason(in.Reason); e != nil {
					return nil, e
				}
				if in.Reason == "" {
					return nil, invalid("reason", "Укажите причину решения")
				}
				if !text(in.Completed, 2000) || !text(in.Obstacles, 2000) {
					return nil, invalid("completed", "Итог и препятствия: до 2000 символов")
				}
				return s.idempotent(r, in, func(tx pgx.Tx) (any, error) {
					ctx := r.Context()
					if e = s.lock(ctx, tx, id); e != nil {
						return nil, e
					}
					g, e := s.get(ctx, tx, id, chi.URLParam(r, "id"))
					if e != nil {
						return nil, e
					}
					if p := platform.MatchVersion(r, g.Version); p != nil {
						return nil, p
					}
					if g.ArchivedAt != nil && action != "restore" {
						return nil, problem(409, "ARCHIVED", "Сначала восстановите цель")
					}
					next := g.State
					archived := g.ArchivedAt
					explanation := in.Reason
					transitionError := problem(409, "INVALID_TRANSITION", "Решение недоступно в текущем состоянии цели")
					switch action {
					case "pause":
						if g.State != "active" {
							return nil, transitionError
						}
						next = "paused"
					case "resume":
						if g.State != "draft" && g.State != "paused" {
							return nil, transitionError
						}
						next = "active"
					case "achieve":
						if g.State != "active" {
							return nil, transitionError
						}
						progress, e := s.calculate(ctx, tx, id, g)
						if e != nil {
							return nil, e
						}
						if progress < 100 {
							return nil, invalid("criteria_confirmed", "Сначала достигните 100% по источникам")
						}
						if !in.CriteriaConfirmed {
							return nil, invalid("criteria_confirmed", "Подтвердите выполнение критерия успеха")
						}
						next = "achieved"
					case "close-unachieved":
						if g.State != "active" && g.State != "paused" {
							return nil, transitionError
						}
						if in.Completed == "" || in.Obstacles == "" {
							return nil, invalid("completed", "Опишите выполненное и препятствия")
						}
						next = "not_achieved"
					case "cancel":
						if closed(g.State) {
							return nil, transitionError
						}
						next = "cancelled"
					case "archive":
						if !closed(g.State) {
							return nil, transitionError
						}
						now := s.clock.Now().UTC()
						archived = &now
					case "restore":
						if archived == nil {
							return nil, transitionError
						}
						archived = nil
					case "review":
						if in.Resolution == "close" {
							if !closed(g.State) {
								return nil, transitionError
							}
							now := s.clock.Now().UTC()
							archived = &now
						} else if in.Resolution == "continue" || in.Resolution == "rebuild" {
							if g.State == "draft" || in.NextInput == nil {
								return nil, invalid("next_input", "Укажите пересмотренный критерий и будущую дату")
							}
							input, e := Validate(*in.NextInput)
							if e != nil {
								return nil, e
							}
							var today string
							e = tx.QueryRow(ctx, `SELECT ($2::timestamptz AT TIME ZONE COALESCE(preferences->>'timezone','UTC'))::date::text FROM user_settings WHERE owner_id=$1`, id, s.clock.Now().UTC()).Scan(&today)
							if e != nil {
								return nil, e
							}
							if input.TargetDate == nil || *input.TargetDate <= today {
								return nil, invalid("next_input", "Новая целевая дата должна быть в будущем")
							}
							if e = s.labels(ctx, tx, id, input); e != nil {
								return nil, e
							}
							if e = s.setupMetric(ctx, tx, id, g.ID, input); e != nil {
								return nil, e
							}
							if in.Resolution == "rebuild" {
								if _, e = tx.Exec(ctx, `DELETE FROM goal_quests WHERE owner_id=$1 AND goal_id=$2`, id, g.ID); e != nil {
									return nil, e
								}
								if _, e = tx.Exec(ctx, `DELETE FROM milestones WHERE owner_id=$1 AND goal_id=$2`, id, g.ID); e != nil {
									return nil, e
								}
							}
							data, e := json.Marshal(input)
							if e != nil {
								return nil, e
							}
							_, e = tx.Exec(ctx, `UPDATE goals SET payload=$3,area_id=$4,start_date=$5::text::date,target_date=$6::text::date WHERE owner_id=$1 AND id=$2`, id, g.ID, data, input.AreaID, input.StartDate, input.TargetDate)
							if e != nil {
								return nil, e
							}
							next = "active"
						} else {
							return nil, invalid("resolution", "Выберите: продолжить, перестроить план или закрыть")
						}
						explanation += "\nРешение пересмотра: " + map[string]string{"continue": "Продолжить с новой датой", "rebuild": "Перестроить будущий план", "close": "Закрыть и убрать в архив"}[in.Resolution]
					}
					if in.Completed != "" {
						explanation += "\nВыполнено: " + in.Completed
					}
					if in.Obstacles != "" {
						explanation += "\nПрепятствия: " + in.Obstacles
					}
					if _, e = tx.Exec(ctx, `UPDATE goals SET state=$3,archived_at=$4 WHERE owner_id=$1 AND id=$2`, id, g.ID, next, archived); e != nil {
						return nil, e
					}
					fresh, e := s.get(ctx, tx, id, g.ID)
					if e != nil {
						return nil, e
					}
					return s.touch(ctx, tx, id, fresh, action, explanation)
				})
			})
		})
	}
	r.Get("/api/v1/goals/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		g, e := s.get(r.Context(), s.pool, id, chi.URLParam(r, "id"))
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		cursor, e := before(r)
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		rows, e := s.pool.Query(r.Context(), `SELECT id,goal_version,kind,reason,snapshot,created_at FROM goal_versions WHERE owner_id=$1 AND goal_id=$2 AND id<$3 ORDER BY id DESC LIMIT 50`, id, g.ID, cursor)
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		defer rows.Close()
		out := []Version{}
		for rows.Next() {
			var v Version
			if e = rows.Scan(&v.ID, &v.GoalVersion, &v.Kind, &v.Reason, &v.Snapshot, &v.CreatedAt); e != nil {
				break
			}
			out = append(out, v)
		}
		if e == nil {
			e = rows.Err()
		}
		s.reply(w, r, out, e)
	})
}
