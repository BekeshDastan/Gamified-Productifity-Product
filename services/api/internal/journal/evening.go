package journal

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type ReviewInput struct {
	Obstacle     string `json:"obstacle"`
	ObstacleNote string `json:"obstacle_note"`
	Conclusion   string `json:"conclusion"`
	Energy       *int   `json:"energy"`
	Mood         *int   `json:"mood"`
}
type DailyItem struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Status         string  `json:"status"`
	PlannedDate    *string `json:"planned_date"`
	Planned        bool    `json:"planned"`
	CompletedToday bool    `json:"completed_today"`
}
type DailyStats struct {
	Planned        int         `json:"planned"`
	Completed      int         `json:"completed"`
	CompletedToday int         `json:"completed_today"`
	Postponed      int         `json:"postponed"`
	Failed         int         `json:"failed"`
	Skipped        int         `json:"skipped"`
	More           bool        `json:"more"`
	Items          []DailyItem `json:"items"`
}
type Review struct {
	Run      *Run       `json:"run"`
	Timezone string     `json:"timezone"`
	Stats    DailyStats `json:"stats"`
}

func validateReview(in ReviewInput) (ReviewInput, error) {
	in.ObstacleNote = strings.TrimSpace(in.ObstacleNote)
	in.Conclusion = strings.TrimSpace(in.Conclusion)
	valid := false
	for _, v := range []string{"none", "insufficient_time", "low_energy", "external", "unclear", "other"} {
		valid = valid || v == in.Obstacle
	}
	if !valid || in.Conclusion == "" || utf8.RuneCountInString(in.Conclusion) > 5000 || utf8.RuneCountInString(in.ObstacleNote) > 2000 || strings.ContainsRune(in.Conclusion+in.ObstacleNote, 0) {
		return in, privateapi.Invalid("Выберите препятствие и запишите вывод до 5000 символов; заметка до 2000")
	}
	for _, v := range []*int{in.Energy, in.Mood} {
		if v != nil && (*v < 1 || *v > 5) {
			return in, privateapi.Invalid("Энергия и настроение: от 1 до 5 или без отметки")
		}
	}
	return in, nil
}
func (s *Service) dailyStats(ctx context.Context, tx pgx.Tx, owner, date, zone string) (DailyStats, error) {
	out := DailyStats{Items: []DailyItem{}}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return out, e
	}
	d, e := time.ParseInLocation("2006-01-02", date, loc)
	if e != nil {
		return out, e
	}
	to := d.AddDate(0, 0, 1)
	plan, e := s.plans.ViewInTx(ctx, tx, owner, date)
	if e != nil {
		return out, e
	}
	ids := []string{}
	if plan.Snapshot != nil {
		for _, item := range plan.Snapshot.Items {
			ids = append(ids, item.ID)
		}
	} else {
		ids = append(ids, plan.QuestIDs...)
	}
	// Counts use the full source set, while details are bounded separately.
	const source = ` FROM quests q WHERE q.owner_id=$1 AND (q.planned_date=$2::text::date OR q.id::text=ANY($3::text[]) OR (q.completed_at>=$4 AND q.completed_at<$5)) AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id)`
	const planned = `(q.planned_date=$2::text::date OR q.id::text=ANY($3::text[]))`
	const completed = `(q.status='completed' AND q.completed_at>=$4 AND q.completed_at<$5)`
	var total int
	e = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE `+planned+`),count(*) FILTER(WHERE `+planned+` AND `+completed+`),count(*) FILTER(WHERE `+completed+`),count(*) FILTER(WHERE `+planned+` AND q.status='postponed'),count(*) FILTER(WHERE `+planned+` AND q.status='failed'),count(*) FILTER(WHERE `+planned+` AND q.status='skipped'),count(*)`+source, owner, date, ids, d, to).Scan(&out.Planned, &out.Completed, &out.CompletedToday, &out.Postponed, &out.Failed, &out.Skipped, &total)
	if e != nil {
		return out, e
	}
	out.More = total > 500
	rows, e := tx.Query(ctx, `SELECT q.id::text,q.payload->>'title',q.status,q.planned_date::text,`+planned+`,COALESCE(`+completed+`,false)`+source+` ORDER BY q.id LIMIT 500`, owner, date, ids, d, to)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v DailyItem
		if e = rows.Scan(&v.ID, &v.Title, &v.Status, &v.PlannedDate, &v.Planned, &v.CompletedToday); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}
func (s *Service) review(r *http.Request, tx pgx.Tx, owner, date string, in *ReviewInput) (Review, error) {
	if e := privateapi.Day(date); e != nil {
		return Review{}, e
	}
	if s.plans == nil {
		return Review{}, privateapi.Problem(503, "UNAVAILABLE", "Планирование недоступно")
	}
	ctx := r.Context()
	var zone string
	if e := tx.QueryRow(ctx, "SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1", owner).Scan(&zone); e != nil {
		return Review{}, e
	}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return Review{}, e
	}
	if date > s.Clock.Now().In(loc).Format("2006-01-02") {
		return Review{}, privateapi.Invalid("Будущий день ещё нельзя подвести")
	}
	run, e := s.run(ctx, tx, owner, date, "evening")
	if e != nil {
		return Review{}, e
	}
	stats, e := s.dailyStats(ctx, tx, owner, date, zone)
	if e != nil {
		return Review{}, e
	}
	if in != nil {
		valid, e := validateReview(*in)
		if e != nil {
			return Review{}, e
		}
		version := int64(0)
		if run != nil {
			version = run.Version
		}
		if p := privateapi.MatchVersion(r, version); p != nil {
			return Review{}, p
		}
		run, e = s.saveRun(ctx, tx, owner, date, "evening", struct {
			Answers  ReviewInput `json:"answers"`
			Stats    DailyStats  `json:"stats"`
			Timezone string      `json:"timezone"`
		}{valid, stats, zone})
		if e != nil {
			return Review{}, e
		}
	}
	return Review{run, zone, stats}, nil
}
func (s *Service) mountEvening(r chi.Router) {
	r.Get("/api/v1/reviews/day/{date}", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			return s.review(r, tx, owner, chi.URLParam(r, "date"), nil)
		})
		s.Reply(w, r, out, e)
	})
	r.Put("/api/v1/reviews/day/{date}", func(w http.ResponseWriter, r *http.Request) {
		privateapi.Decode(s.Service, w, r, func(in ReviewInput) (any, error) {
			return s.Mutate(r, in, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
				return s.review(r, tx, owner, chi.URLParam(r, "date"), &in)
			})
		})
	})
}
