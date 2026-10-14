package planning

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type RadarTask struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Version          int64     `json:"version"`
	Deadline         time.Time `json:"deadline"`
	EstimatedMinutes *int      `json:"estimated_minutes"`
	URL              string    `json:"url"`
}
type RadarBucket struct {
	ID                string      `json:"id"`
	Label             string      `json:"label"`
	Before            time.Time   `json:"before"`
	Items             []RadarTask `json:"items"`
	RequiredMinutes   int         `json:"required_minutes"`
	CumulativeMinutes int         `json:"cumulative_minutes"`
	FreeMinutes       int         `json:"free_minutes"`
	DeficitMinutes    int         `json:"deficit_minutes"`
	UnknownEstimates  int         `json:"unknown_estimates"`
}
type Radar struct {
	Timezone   string        `json:"timezone"`
	ComputedAt time.Time     `json:"computed_at"`
	Buckets    []RadarBucket `json:"buckets"`
	Notice     string        `json:"notice"`
}

func radarIndex(deadline, now time.Time) int {
	if deadline.Before(now) {
		return 0
	}
	if deadline.Before(now.Add(24 * time.Hour)) {
		return 1
	}
	if !deadline.After(now.Add(72 * time.Hour)) {
		return 2
	}
	if !deadline.After(now.Add(7 * 24 * time.Hour)) {
		return 3
	}
	return -1
}
func (s *Service) radar(ctx context.Context, tx pgx.Tx, owner string) (Radar, error) {
	now := s.clock.Now()
	out := Radar{ComputedAt: now, Buckets: []RadarBucket{}, Notice: "Свободные окна считаются от текущего момента. Каждая группа сравнивает суммарную оценку этой и более срочных групп с доступностью до своей границы; ёмкости групп не складываются. Неизвестные оценки не заменяются нулём. Это арифметика, без вероятностей исхода."}
	if e := tx.QueryRow(ctx, `SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1 FOR SHARE`, owner).Scan(&out.Timezone); e != nil {
		return out, e
	}
	zone, e := time.LoadLocation(out.Timezone)
	if e != nil {
		return out, e
	}
	for _, v := range []struct {
		id, label string
		end       time.Time
	}{{"overdue", "Просрочено", now}, {"day", "Менее суток", now.Add(24 * time.Hour)}, {"three_days", "От суток до трёх дней", now.Add(72 * time.Hour)}, {"week", "Больше трёх дней, до недели", now.Add(7 * 24 * time.Hour)}} {
		out.Buckets = append(out.Buckets, RadarBucket{ID: v.id, Label: v.label, Before: v.end, Items: []RadarTask{}})
	}
	rows, e := tx.Query(ctx, `SELECT q.id::text,q.payload->>'title',q.version,q.deadline,NULLIF(COALESCE((q.payload->>'estimated_minutes')::int,0),0) FROM quests q WHERE q.owner_id=$1 AND q.deadline<=$2 AND q.archived_at IS NULL AND q.status NOT IN('completed','failed','skipped') AND COALESCE((q.payload->>'required')::boolean,true) AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id) ORDER BY q.deadline,q.id LIMIT 1001`, owner, now.Add(7*24*time.Hour))
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var q RadarTask
		if e = rows.Scan(&q.ID, &q.Title, &q.Version, &q.Deadline, &q.EstimatedMinutes); e != nil {
			rows.Close()
			return out, e
		}
		index := radarIndex(q.Deadline, now)
		if index < 0 {
			continue
		}
		q.URL = "/app/quests/" + q.ID
		b := &out.Buckets[index]
		b.Items = append(b.Items, q)
		if q.EstimatedMinutes == nil {
			b.UnknownEstimates++
		} else {
			b.RequiredMinutes += *q.EstimatedMinutes
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	count, total := 0, 0
	for i := range out.Buckets {
		b := &out.Buckets[i]
		count += len(b.Items)
		total += b.RequiredMinutes
		b.CumulativeMinutes = total
		if i > 0 {
			date := time.Date(now.In(zone).Year(), now.In(zone).Month(), now.In(zone).Day(), 0, 0, 0, 0, zone)
			for !date.After(b.Before.In(zone)) {
				slots, e := s.FreeWorkInTx(ctx, tx, owner, date.Format(time.DateOnly), now, b.Before)
				if e != nil {
					return out, e
				}
				b.FreeMinutes += minutes(slots)
				date = date.AddDate(0, 0, 1)
			}
		}
		b.DeficitMinutes = max(0, total-b.FreeMinutes)
	}
	if count > 1000 {
		return out, privateapi.Invalid("Слишком много срочных задач. Уточните сроки")
	}
	return out, nil
}
func (s *Service) mountRadar(r chi.Router) {
	api := privateapi.New(s.pool, s.clock)
	r.Get("/api/v1/plans/deadline-radar", func(w http.ResponseWriter, r *http.Request) {
		out, e := api.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) { return s.radar(ctx, tx, owner) })
		api.Reply(w, r, out, e)
	})
}
