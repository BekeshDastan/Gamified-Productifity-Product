package quests

import (
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"unicode/utf8"
)

func (s *Service) validateParent(ctx context.Context, tx pgx.Tx, id, key string, parent *string) error {
	if parent == nil {
		return nil
	}
	if !uuid(*parent) || *parent == key {
		return invalid("parent_id", "Нельзя сделать задачу своим родителем")
	}
	p, err := get(ctx, tx, id, *parent, false)
	if err != nil {
		return invalid("parent_id", "Родитель не найден")
	}
	if p.ArchivedAt != nil {
		return invalid("parent_id", "Сначала восстановите родительскую задачу")
	}
	var oldParent *string
	if key != "" {
		if err := tx.QueryRow(ctx, `SELECT parent_id::text FROM quests WHERE owner_id=$1 AND id=$2`, id, key).Scan(&oldParent); err != nil {
			return err
		}
	}
	if p.Status == "completed" && (oldParent == nil || *oldParent != *parent) {
		return invalid("parent_id", "Сначала откройте родителя снова")
	}
	depth := 1
	ancestor := p.ParentID
	for ancestor != nil {
		if *ancestor == key {
			return invalid("parent_id", "Цикл в дереве задач")
		}
		p, err = get(ctx, tx, id, *ancestor, false)
		if err != nil {
			return err
		}
		depth++
		ancestor = p.ParentID
		if depth > 2 {
			return invalid("parent_id", "Не более двух уровней подзадач")
		}
	}
	if key != "" {
		var height int
		err = tx.QueryRow(ctx, `WITH RECURSIVE tree AS (
 SELECT id,0 depth FROM quests WHERE owner_id=$1 AND id=$2
 UNION ALL SELECT q.id,t.depth+1 FROM quests q JOIN tree t ON q.parent_id=t.id WHERE q.owner_id=$1)
 SELECT COALESCE(max(depth),0) FROM tree`, id, key).Scan(&height)
		if err != nil {
			return err
		}
		if height+depth > 2 {
			return invalid("parent_id", "Перенос создаст больше двух уровней")
		}
		cycle, err := dependencyPath(ctx, tx, id, key, *parent)
		if err != nil {
			return err
		}
		if cycle {
			return invalid("parent_id", "Родительство создаст цикл блокировок")
		}
	}
	return nil
}

const graphEdges = `SELECT quest_id a,depends_on b FROM quest_dependencies WHERE owner_id=$1
 UNION SELECT parent_id a,id b FROM quests WHERE owner_id=$1 AND parent_id IS NOT NULL AND COALESCE((payload->>'required')::boolean,true)`

func dependencyPath(ctx context.Context, tx pgx.Tx, id, from, to string) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, `WITH RECURSIVE edges AS (`+graphEdges+`), reach(id) AS (
 SELECT $2::uuid UNION SELECT e.b FROM edges e JOIN reach r ON e.a=r.id)
 SELECT EXISTS(SELECT 1 FROM reach WHERE id=$3::uuid)`, id, from, to).Scan(&found)
	return found, err
}

type Summary struct {
	Container       bool `json:"container"`
	RequiredLeaves  int  `json:"required_leaves"`
	CompletedLeaves int  `json:"completed_leaves"`
}

func summary(ctx context.Context, tx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id, key string) (Summary, error) {
	var result Summary
	err := tx.QueryRow(ctx, `WITH RECURSIVE tree AS (
 SELECT id,status,COALESCE((payload->>'required')::boolean,true) mandatory FROM quests WHERE owner_id=$1 AND parent_id=$2
 UNION ALL SELECT q.id,q.status,t.mandatory AND COALESCE((q.payload->>'required')::boolean,true) FROM quests q JOIN tree t ON q.parent_id=t.id WHERE q.owner_id=$1)
 SELECT EXISTS(SELECT 1 FROM tree),
 count(*) FILTER(WHERE mandatory AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=$1 AND c.parent_id=tree.id)),
 count(*) FILTER(WHERE mandatory AND status='completed' AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=$1 AND c.parent_id=tree.id)) FROM tree`, id, key).Scan(&result.Container, &result.RequiredLeaves, &result.CompletedLeaves)
	return result, err
}
func (s *Service) checkBlocked(ctx context.Context, tx pgx.Tx, id, key string) error {
	var blocked bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quest_dependencies d JOIN quests q ON q.id=d.depends_on AND q.owner_id=d.owner_id WHERE d.owner_id=$1 AND d.quest_id=$2 AND q.status<>'completed')`, id, key).Scan(&blocked)
	if err != nil {
		return err
	}
	if blocked {
		return problem(409, "DEPENDENCY_BLOCKED", "Сначала завершите блокирующие задачи")
	}
	return nil
}

// Bottom-up rollups only change containers, never complete or reopen children.
func (s *Service) rollup(ctx context.Context, tx pgx.Tx, id, key string) error {
	for level := 0; key != "" && level < 4; level++ {
		q, err := get(ctx, tx, id, key, true)
		if err != nil {
			return err
		}
		sum, err := summary(ctx, tx, id, key)
		if err != nil {
			return err
		}
		next := q.Status
		kind := "rollup_completed"
		if sum.Container && sum.RequiredLeaves > 0 {
			if sum.CompletedLeaves == sum.RequiredLeaves {
				block := s.checkBlocked(ctx, tx, id, key)
				if block == nil {
					next = "completed"
				} else {
					if p, ok := block.(*platform.APIError); !ok || p.Code != "DEPENDENCY_BLOCKED" {
						return block
					}
					if next == "completed" {
						next = "todo"
						kind = "rollup_reopened"
					}
				}
			} else if next == "completed" {
				next = "todo"
				kind = "rollup_reopened"
			}
		}
		if next != q.Status {
			before := q
			q.Status = next
			q.CompletedAt = nil
			if next == "completed" {
				now := s.clock.Now().UTC()
				q.CompletedAt = &now
			}
			data, err := json.Marshal(q.Input)
			if err != nil {
				return err
			}
			q, err = scan(tx.QueryRow(ctx, `UPDATE quests SET payload=$3,status=$4,completed_at=$5,version=version+1,updated_at=$6 WHERE owner_id=$1 AND id=$2 RETURNING `+columns, id, key, data, next, q.CompletedAt, s.clock.Now().UTC()))
			if err != nil {
				return err
			}
			if err := s.event(ctx, tx, id, kind, "Прогресс обязательных подзадач", &before, &q); err != nil {
				return err
			}
			if err := s.rollupDependents(ctx, tx, id, q.ID); err != nil {
				return err
			}
		}
		if q.ParentID == nil {
			break
		}
		key = *q.ParentID
	}
	return nil
}
func (s *Service) rollupDependents(ctx context.Context, tx pgx.Tx, id, key string) error {
	rows, err := tx.Query(ctx, `SELECT quest_id::text FROM quest_dependencies WHERE owner_id=$1 AND depends_on=$2 ORDER BY quest_id`, id, key)
	if err != nil {
		return err
	}
	keys := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		keys = append(keys, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range keys {
		if err := s.rollup(ctx, tx, id, v); err != nil {
			return err
		}
	}
	return nil
}

type Structure struct {
	Summary
	Children     []Quest `json:"children"`
	Dependencies []Quest `json:"dependencies"`
}
type DependenciesInput struct {
	IDs    []string `json:"ids"`
	Reason string   `json:"reason"`
}

func (s *Service) mountStructure(r chi.Router) {
	r.Get("/api/v1/quests/{id}/structure", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		if e != nil {
			s.reply(w, r, nil, e)
			return
		}
		key := chi.URLParam(r, "id")
		var out Structure
		out.Children = []Quest{}
		out.Dependencies = []Quest{}
		e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
			if _, e := get(r.Context(), tx, id, key, false); e != nil {
				return e
			}
			var e error
			out.Summary, e = summary(r.Context(), tx, id, key)
			if e != nil {
				return e
			}
			for _, kind := range []string{"children", "dependencies"} {
				sql := `SELECT ` + columns + ` FROM quests WHERE owner_id=$1 AND parent_id=$2 ORDER BY created_at,id`
				if kind == "dependencies" {
					sql = `SELECT ` + columns + ` FROM quests WHERE owner_id=$1 AND id IN(SELECT depends_on FROM quest_dependencies WHERE owner_id=$1 AND quest_id=$2) ORDER BY created_at,id`
				}
				rows, e := tx.Query(r.Context(), sql, id, key)
				if e != nil {
					return e
				}
				for rows.Next() {
					q, e := scan(rows)
					if e != nil {
						rows.Close()
						return e
					}
					if kind == "children" {
						out.Children = append(out.Children, q)
					} else {
						out.Dependencies = append(out.Dependencies, q)
					}
				}
				e = rows.Err()
				rows.Close()
				if e != nil {
					return e
				}
			}
			return nil
		})
		s.reply(w, r, out, e)
	})
	r.Put("/api/v1/quests/{id}/dependencies", func(w http.ResponseWriter, r *http.Request) {
		decode(w, r, func(in DependenciesInput) (any, error) {
			id, e := owner(r.Context())
			if e != nil {
				return nil, e
			}
			key := chi.URLParam(r, "id")
			if in.IDs == nil {
				in.IDs = []string{}
			}
			if len(in.IDs) > 50 {
				return nil, invalid("ids", "Не более 50 блокировок")
			}
			seen := map[string]bool{}
			for _, v := range in.IDs {
				if !uuid(v) || v == key || seen[v] {
					return nil, invalid("ids", "Выберите разные задачи")
				}
				seen[v] = true
			}
			in.Reason = strings.TrimSpace(in.Reason)
			if utf8.RuneCountInString(in.Reason) > 1000 || strings.ContainsRune(in.Reason, 0) {
				return nil, invalid("reason", "До 1000 символов")
			}
			var q Quest
			e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
				if e := s.lockOwner(r.Context(), tx, id); e != nil {
					return e
				}
				current, e := get(r.Context(), tx, id, key, true)
				if e != nil {
					return e
				}
				if p := platform.MatchVersion(r, current.Version); p != nil {
					return p
				}
				if current.ArchivedAt != nil {
					return problem(409, "ARCHIVED", "Сначала восстановите задачу")
				}
				var removed bool
				e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM quest_dependencies WHERE owner_id=$1 AND quest_id=$2 AND NOT(depends_on=ANY($3::uuid[])))`, id, key, in.IDs).Scan(&removed)
				if e != nil {
					return e
				}
				if removed && in.Reason == "" {
					return invalid("reason", "Укажите причину снятия блокировки")
				}
				var previousIDs, previousTitles []string
				e = tx.QueryRow(r.Context(), `SELECT ARRAY(SELECT depends_on::text FROM quest_dependencies WHERE owner_id=$1 AND quest_id=$2 ORDER BY depends_on),
				ARRAY(SELECT q.payload->>'title' FROM quest_dependencies d JOIN quests q ON q.owner_id=d.owner_id AND q.id=d.depends_on WHERE d.owner_id=$1 AND d.quest_id=$2 ORDER BY d.depends_on)`, id, key).Scan(&previousIDs, &previousTitles)
				if e != nil {
					return e
				}
				titles := []string{}
				for _, v := range in.IDs {
					dep, e := get(r.Context(), tx, id, v, false)
					if e != nil || dep.ArchivedAt != nil {
						return invalid("ids", "Блокирующая задача не найдена")
					}
					cycle, e := dependencyPath(r.Context(), tx, id, v, key)
					if e != nil {
						return e
					}
					if cycle {
						return invalid("ids", "Цикл зависимостей")
					}
					if (current.Status == "in_progress" || current.Status == "completed") && dep.Status != "completed" {
						return invalid("ids", "Сначала верните задачу к выполнению")
					}
					titles = append(titles, dep.Title)
				}
				if _, e := tx.Exec(r.Context(), `DELETE FROM quest_dependencies WHERE owner_id=$1 AND quest_id=$2`, id, key); e != nil {
					return e
				}
				for _, v := range in.IDs {
					if _, e := tx.Exec(r.Context(), `INSERT INTO quest_dependencies(owner_id,quest_id,depends_on) VALUES($1,$2,$3)`, id, key, v); e != nil {
						return e
					}
				}
				q, e = scan(tx.QueryRow(r.Context(), `UPDATE quests SET version=version+1,updated_at=$3 WHERE owner_id=$1 AND id=$2 RETURNING `+columns, id, key, s.clock.Now().UTC()))
				if e == nil {
					e = s.event(r.Context(), tx, id, "dependencies", in.Reason, &current, &q)
				}
				if e == nil {
					details, err := json.Marshal(map[string][]string{"previous_ids": previousIDs, "ids": in.IDs, "previous_titles": previousTitles, "titles": titles})
					if err != nil {
						return err
					}
					_, e = tx.Exec(r.Context(), `UPDATE task_events SET details=$4 WHERE owner_id=$1 AND quest_id=$2 AND quest_version=$3`, id, key, q.Version, details)
				}
				if e == nil {
					e = s.rollup(r.Context(), tx, id, key)
				}
				if e == nil {
					q, e = get(r.Context(), tx, id, key, false)
				}
				return e
			})
			return q, e
		}, s)
	})
}
