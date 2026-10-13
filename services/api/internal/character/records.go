package character

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"sort"
	"strings"
	"time"
)

type RecordSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}
type Record struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Value       *int64         `json:"value"`
	Unit        string         `json:"unit"`
	Available   bool           `json:"available"`
	Explanation string         `json:"explanation"`
	Sources     []RecordSource `json:"sources"`
}

func number(v int64) *int64 { return &v }
func (s *Service) records(ctx context.Context, tx pgx.Tx, owner string) ([]Record, error) {
	f, evidence, ready, e := s.facts(ctx, tx, owner, false)
	if e != nil {
		return nil, e
	}
	streak := Record{ID: "best-streak", Name: "Лучшая серия привычки", Unit: "запланированных норм", Available: ready, Explanation: "Полные нормы подряд. Дни вне расписания и отдых не разрывают серию.", Sources: []RecordSource{}}
	if ready {
		streak.Value = number(f.Values[BestStreak])
		for _, h := range evidence {
			if int64(h.Stats.Best) == f.Values[BestStreak] && h.Stats.Best > 0 {
				streak.Sources = append(streak.Sources, RecordSource{h.Title, "/app/habits#habit-" + h.ID})
				break
			}
		}
	} else {
		streak.Explanation = "Догружаем историю привычек. Рекорд появится после проверки всех периодов."
	}
	important := Record{ID: "important-tasks", Name: "Важные задачи выполнены", Value: number(f.Values[ImportantTasks]), Unit: "самостоятельных задач", Available: true, Explanation: "Завершённые задачи высокого приоритета, без контейнеров. Последние пять источников:", Sources: []RecordSource{}}
	rows, e := tx.Query(ctx, "SELECT q.id::text,q.payload->>'title' FROM quests q WHERE owner_id=$1 AND status='completed' AND payload->>'priority'='high' AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id) ORDER BY completed_at DESC,id DESC LIMIT 5", owner)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var id, title string
		if e = rows.Scan(&id, &title); e != nil {
			rows.Close()
			return nil, e
		}
		important.Sources = append(important.Sources, RecordSource{title, "/app/quests/" + id})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	weekly := Record{ID: "weekly-regularity", Name: "Лучшая недельная регулярность", Unit: "%", Available: ready, Explanation: "Только полностью прошедшие местные недели и ежедневные нормы одной привычки. Недельные и месячные нормы не превращаются в дни.", Sources: []RecordSource{}}
	best := -1
	if ready {
		for _, h := range evidence {
			type week struct {
				start          time.Time
				success, total int
			}
			weeks := map[string]*week{}
			for _, p := range h.Periods {
				if p.Excluded || !strings.HasPrefix(p.Key, "daily:") {
					continue
				}
				zone, e := time.LoadLocation(p.Timezone)
				if e != nil {
					return nil, e
				}
				a := p.Start.In(zone)
				a = time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, zone)
				offset := (int(a.Weekday()) + 6) % 7
				monday := a.AddDate(0, 0, -offset)
				if s.clock.Now().Before(monday.AddDate(0, 0, 7)) {
					continue
				}
				key := monday.Format(time.DateOnly)
				if weeks[key] == nil {
					weeks[key] = &week{start: monday}
				}
				weeks[key].total++
				if p.Amount >= p.Target {
					weeks[key].success++
				}
			}
			keys := []string{}
			for key := range weeks {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				w := weeks[key]
				rate := w.success * 100 / w.total
				if rate > best {
					best = rate
					weekly.Value = number(int64(rate))
					weekly.Explanation = fmt.Sprintf("Неделя с %s: %d из %d запланированных ежедневных норм. Пояс %s.", key, w.success, w.total, w.start.Location())
					weekly.Sources = []RecordSource{{h.Title, "/app/habits#habit-" + h.ID}}
				}
			}
		}
	}
	focus := Record{ID: "longest-focus", Name: "Самая длинная подтверждённая фокус-сессия", Unit: "минут", Available: true, Value: number(0), Explanation: "Только подтверждённые рабочие интервалы, без пауз, перерывов и отмеченного отдыха.", Sources: []RecordSource{}}
	var focusID, focusTitle string
	var focusSeconds int64
	e = tx.QueryRow(ctx, `SELECT f.id::text,f.title,s.seconds FROM focus_sessions f CROSS JOIN LATERAL (SELECT COALESCE(sum(floor(extract(epoch FROM(ended_at-started_at)))),0)::bigint seconds FROM focus_segments WHERE owner_id=f.owner_id AND session_id=f.id AND kind='work' AND ended_at IS NOT NULL) s WHERE f.owner_id=$1 AND f.state='finished' AND f.result<>'rest' AND s.seconds>0 ORDER BY s.seconds DESC,f.started_at,f.id LIMIT 1`, owner).Scan(&focusID, &focusTitle, &focusSeconds)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return nil, e
	}
	if e == nil {
		focus.Value = number(focusSeconds / 60)
		focus.Sources = append(focus.Sources, RecordSource{focusTitle, "/app/focus?session_id=" + focusID})
	}

	return []Record{streak, focus, important, weekly}, nil
}
