package planning

import (
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type Dashboard struct {
	Date          string           `json:"date"`
	Timezone      string           `json:"timezone"`
	Plan          Plan             `json:"plan"`
	Tasks         []quests.Quest   `json:"tasks"`
	Deadlines     []quests.Quest   `json:"deadlines"`
	Events        []calendar.Event `json:"events"`
	MoreTasks     bool             `json:"more_tasks"`
	MoreDeadlines bool             `json:"more_deadlines"`
	MoreEvents    bool             `json:"more_events"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

func dashboardTasks(ctx context.Context, tx pgx.Tx, id, condition string, args ...any) ([]quests.Quest, bool, error) {
	values := append([]any{id}, args...)
	rows, e := tx.Query(ctx, `SELECT q.id::text,q.payload,q.version,q.status,q.completed_at,q.archived_at,q.created_at,q.updated_at FROM quests q WHERE q.owner_id=$1 AND q.archived_at IS NULL AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id AND c.archived_at IS NULL) AND (`+condition+`) ORDER BY q.deadline NULLS LAST,q.created_at,q.id LIMIT 101`, values...)
	if e != nil {
		return nil, false, e
	}
	defer rows.Close()
	out := []quests.Quest{}
	for rows.Next() {
		var q quests.Quest
		var payload []byte
		var status string
		if e = rows.Scan(&q.ID, &payload, &q.Version, &status, &q.CompletedAt, &q.ArchivedAt, &q.CreatedAt, &q.UpdatedAt); e != nil {
			return nil, false, e
		}
		if e = json.Unmarshal(payload, &q.Input); e != nil {
			return nil, false, e
		}
		q.Status = status
		out = append(out, q)
	}
	if e = rows.Err(); e != nil {
		return nil, false, e
	}
	more := len(out) > 100
	if more {
		out = out[:100]
	}
	return out, more, nil
}
func (s *Service) dashboard(ctx context.Context, tx pgx.Tx, id, date string) (Dashboard, error) {
	out := Dashboard{Tasks: []quests.Quest{}, Deadlines: []quests.Quest{}, Events: []calendar.Event{}, UpdatedAt: s.clock.Now().UTC()}
	if e := s.lock(ctx, tx, id); e != nil {
		return out, e
	}
	if e := tx.QueryRow(ctx, `SELECT preferences->>'timezone' FROM user_settings WHERE owner_id=$1`, id).Scan(&out.Timezone); e != nil {
		return out, e
	}
	zone, e := time.LoadLocation(out.Timezone)
	if e != nil {
		return out, e
	}
	if date == "" {
		date = s.clock.Now().In(zone).Format(time.DateOnly)
	}
	d, e := day(date)
	if e != nil {
		return out, e
	}
	out.Date = date
	out.Plan, e = s.get(ctx, tx, id, date)
	if e != nil {
		return out, e
	}
	out.Tasks, out.MoreTasks, e = dashboardTasks(ctx, tx, id, `q.planned_date=$2::text::date OR q.id IN(SELECT quest_id FROM day_plan_items WHERE owner_id=$1 AND plan_id=$3::uuid)`, date, nilUUID(out.Plan.ID))
	if e != nil {
		return out, e
	}
	a, _, e := bounds(date, out.Timezone)
	if e != nil {
		return out, e
	}
	_, b, e := bounds(d.AddDate(0, 0, 6).Format(time.DateOnly), out.Timezone)
	if e != nil {
		return out, e
	}
	out.Deadlines, out.MoreDeadlines, e = dashboardTasks(ctx, tx, id, `q.status NOT IN ('completed','failed','skipped') AND q.deadline<$2`, b)
	if e != nil {
		return out, e
	}
	out.Events, e = s.calendar.PreviewTimedInTx(ctx, tx, id, d, d.AddDate(0, 0, 7), a, b)
	if e != nil {
		return out, e
	}
	out.MoreEvents = len(out.Events) > 50
	if out.MoreEvents {
		out.Events = out.Events[:50]
	}
	return out, nil
}
func nilUUID(id string) any {
	if id == "" {
		return nil
	}
	return id
}
func (s *Service) mountDashboard(r chi.Router) {
	r.Get("/api/v1/dashboard", func(w http.ResponseWriter, r *http.Request) {
		id, e := owner(r.Context())
		var out Dashboard
		if e == nil {
			e = storage.WithinTx(r.Context(), s.pool, func(tx pgx.Tx) error {
				var err error
				out, err = s.dashboard(r.Context(), tx, id, r.URL.Query().Get("date"))
				return err
			})
		}
		s.reply(w, r, out, e)
	})
}
