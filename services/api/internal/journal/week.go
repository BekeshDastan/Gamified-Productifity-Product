package journal

import (
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type WeekItem struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Version          int64      `json:"version"`
	Status           string     `json:"status"`
	PlannedDate      *string    `json:"planned_date"`
	Deadline         *time.Time `json:"deadline"`
	EstimatedMinutes int        `json:"estimated_minutes"`
	Priority         string     `json:"priority"`
	Reason           string     `json:"reason"`
}
type ReasonCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}
type WeekSnapshot struct {
	Week          string          `json:"week"`
	NextWeek      string          `json:"next_week"`
	Timezone      string          `json:"timezone"`
	Items         []WeekItem      `json:"items"`
	More          bool            `json:"more"`
	Reasons       []ReasonCount   `json:"reasons"`
	Plans         []planning.Plan `json:"plans"`
	ReviewVersion int64           `json:"review_version"`
}
type WeekPreview struct {
	ID        string       `json:"id"`
	Snapshot  WeekSnapshot `json:"snapshot"`
	ExpiresAt time.Time    `json:"expires_at"`
	Applied   bool         `json:"applied"`
}
type Week struct {
	Snapshot   WeekSnapshot    `json:"snapshot"`
	LastResult json.RawMessage `json:"last_result"`
}

func (s *Service) week(ctx context.Context, tx pgx.Tx, owner, date string) (Week, error) {
	out := Week{Snapshot: WeekSnapshot{Week: date, Items: []WeekItem{}, Reasons: []ReasonCount{}, Plans: []planning.Plan{}}, LastResult: json.RawMessage("null")}
	if e := privateapi.Day(date); e != nil {
		return out, e
	}
	var start int
	var zone string
	e := tx.QueryRow(ctx, "SELECT preferences->>'timezone',(preferences->>'week_start')::int FROM user_settings WHERE owner_id=$1", owner).Scan(&zone, &start)
	if e != nil {
		return out, e
	}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return out, e
	}
	d, e := time.ParseInLocation("2006-01-02", date, loc)
	if e != nil {
		return out, e
	}
	weekday := int(d.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	if weekday != start {
		return out, privateapi.Invalid("Выберите первый день недели согласно настройкам аккаунта")
	}
	out.Snapshot.Timezone = zone
	out.Snapshot.NextWeek = d.AddDate(0, 0, 7).Format("2006-01-02")
	end := d.AddDate(0, 0, 14)
	if e = tx.QueryRow(ctx, "SELECT COALESCE((SELECT version FROM weekly_reviews WHERE owner_id=$1 AND local_date=$2::text::date),0),COALESCE((SELECT payload FROM weekly_reviews WHERE owner_id=$1 AND local_date=$2::text::date),'null'::jsonb)", owner, date).Scan(&out.Snapshot.ReviewVersion, &out.LastResult); e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT q.id::text,q.payload->>'title',q.version,q.status,q.planned_date::text,q.deadline,COALESCE((q.payload->>'estimated_minutes')::int,0),q.payload->>'priority',COALESCE((SELECT reason FROM task_events e WHERE e.owner_id=q.owner_id AND e.quest_id=q.id AND e.reason<>'' ORDER BY e.id DESC LIMIT 1),'') FROM quests q WHERE q.owner_id=$1 AND q.archived_at IS NULL AND q.status IN('backlog','todo','in_progress','postponed') AND (q.planned_date IS NULL OR q.planned_date<$2::text::date) AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id AND c.archived_at IS NULL) ORDER BY q.planned_date NULLS LAST,q.deadline NULLS LAST,q.id LIMIT 501`, owner, end.Format("2006-01-02"))
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var item WeekItem
		if e = rows.Scan(&item.ID, &item.Title, &item.Version, &item.Status, &item.PlannedDate, &item.Deadline, &item.EstimatedMinutes, &item.Priority, &item.Reason); e != nil {
			rows.Close()
			return out, e
		}
		out.Snapshot.Items = append(out.Snapshot.Items, item)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Snapshot.Items) > 500 {
		out.Snapshot.More = true
		out.Snapshot.Items = out.Snapshot.Items[:500]
	}
	rows, e = tx.Query(ctx, `SELECT payload#>>'{answers,obstacle}',count(*) FROM daily_reviews WHERE owner_id=$1 AND local_date>=$2::text::date AND local_date<$3::text::date GROUP BY payload#>>'{answers,obstacle}' ORDER BY 1`, owner, date, out.Snapshot.NextWeek)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var v ReasonCount
		if e = rows.Scan(&v.Reason, &v.Count); e != nil {
			rows.Close()
			return out, e
		}
		out.Snapshot.Reasons = append(out.Snapshot.Reasons, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for i := 7; i < 14; i++ {
		p, e := s.plans.ViewInTx(ctx, tx, owner, d.AddDate(0, 0, i).Format("2006-01-02"))
		if e != nil {
			return out, e
		}
		out.Snapshot.Plans = append(out.Snapshot.Plans, p)
	}
	return out, nil
}
func (s *Service) previewWeek(r *http.Request, tx pgx.Tx, owner, date string) (WeekPreview, error) {
	out, e := s.week(r.Context(), tx, owner, date)
	if e != nil {
		return WeekPreview{}, e
	}
	body, e := json.Marshal(out.Snapshot)
	if e != nil {
		return WeekPreview{}, e
	}
	now := s.Clock.Now().UTC()
	preview := WeekPreview{Snapshot: out.Snapshot, ExpiresAt: now.Add(10 * time.Minute)}
	e = tx.QueryRow(r.Context(), `INSERT INTO weekly_review_previews(owner_id,local_date,snapshot,created_at,expires_at) VALUES($1,$2::text::date,$3,$4,$5) RETURNING id::text`, owner, date, body, now, preview.ExpiresAt).Scan(&preview.ID)
	return preview, e
}
func (s *Service) mountWeek(r chi.Router) {
	r.Get("/api/v1/reviews/week/{date}", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.week(ctx, tx, owner, chi.URLParam(r, "date"))
		})
		s.Reply(w, r, out, e)
	})
	r.Post("/api/v1/reviews/week/{date}/preview", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in struct{}) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.previewWeek(r, tx, owner, chi.URLParam(r, "date"))
			})
		})
	})
}
