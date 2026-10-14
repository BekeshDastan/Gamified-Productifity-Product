package analytics

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/focus"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"sort"
	"strconv"
	"time"
)

type TaskTime struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	URL               string    `json:"url"`
	EstimatedMinutes  *int      `json:"estimated_minutes"`
	ActualSeconds     *int64    `json:"actual_seconds"`
	FocusSeconds      int64     `json:"focus_seconds"`
	ManualMinutes     *int64    `json:"manual_minutes"`
	Mode              string    `json:"mode"`
	ConfirmedSessions int64     `json:"confirmed_sessions"`
	CompletedAt       time.Time `json:"completed_at"`
	TemplateID        *string   `json:"template_id"`
	TemplateName      *string   `json:"template_name"`
	TemplateVersion   *int64    `json:"template_version"`
	AreaID            *string   `json:"area_id"`
}
type TimeTotals struct {
	EstimatedTasks             int   `json:"estimated_tasks"`
	ActualKnownTasks           int   `json:"actual_known_tasks"`
	ComparableTasks            int   `json:"comparable_tasks"`
	ComparableEstimatedSeconds int64 `json:"comparable_estimated_seconds"`
	ComparableActualSeconds    int64 `json:"comparable_actual_seconds"`
}
type TimeMetrics struct {
	TimeTotals
	TaskTimes []TaskTime `json:"task_times"`
}
type TemplateSample struct {
	ID                      string   `json:"id"`
	Name                    string   `json:"name"`
	Version                 int64    `json:"version"`
	Count                   int      `json:"count"`
	AverageEstimatedMinutes *float64 `json:"average_estimated_minutes"`
	AverageActualSeconds    *float64 `json:"average_actual_seconds"`
	Factor                  *float64 `json:"factor"`
	Sufficient              bool     `json:"sufficient"`
	QuestIDs                []string `json:"quest_ids"`
}

func (s *Service) taskTimes(ctx context.Context, tx pgx.Tx, owner string, in Range, timezone string) ([]TaskTime, error) {
	a, b, e := validateRangeLimit(in, 380)
	if e != nil {
		return nil, e
	}
	zone, e := time.LoadLocation(timezone)
	if e != nil {
		return nil, e
	}
	start := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, zone)
	end := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, zone)
	rows, e := tx.Query(ctx, `SELECT q.id::text,q.payload->>'title',q.completed_at,q.area_id::text,COALESCE((SELECT (after_state->>'estimated_minutes')::int FROM task_events WHERE owner_id=q.owner_id AND quest_id=q.id AND kind='complete' ORDER BY id DESC LIMIT 1),(q.payload->>'estimated_minutes')::int,0),o.template_id::text,o.template_name,o.template_version FROM quests q LEFT JOIN task_template_origins o ON o.owner_id=q.owner_id AND o.quest_id=q.id WHERE q.owner_id=$1 AND q.status='completed' AND q.completed_at>=$2 AND q.completed_at<$3 AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id) ORDER BY q.completed_at,q.id`, owner, start, end)
	if e != nil {
		return nil, e
	}
	out := []TaskTime{}
	ids := []string{}
	for rows.Next() {
		v := TaskTime{}
		var estimate int
		if e = rows.Scan(&v.ID, &v.Title, &v.CompletedAt, &v.AreaID, &estimate, &v.TemplateID, &v.TemplateName, &v.TemplateVersion); e != nil {
			rows.Close()
			return nil, e
		}
		if estimate > 0 {
			v.EstimatedMinutes = &estimate
		}
		v.URL = "/app/quests/" + v.ID
		out = append(out, v)
		ids = append(ids, v.ID)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	actual, e := focus.QuestTimesInTx(ctx, tx, owner, ids)
	if e != nil {
		return nil, e
	}
	for i := range out {
		v := &out[i]
		a := actual[v.ID]
		v.Mode = a.Mode
		v.FocusSeconds = a.FocusSeconds
		v.ConfirmedSessions = a.ConfirmedSessions
		if a.Overridden {
			v.ManualMinutes = &a.ManualMinutes
			v.ActualSeconds = &a.EffectiveSeconds
		} else if a.ConfirmedSessions > 0 {
			v.ActualSeconds = &a.EffectiveSeconds
		}
	}
	return out, nil
}
func (s *Service) timeMetrics(ctx context.Context, tx pgx.Tx, owner string, in Range, timezone string, days []Day) error {
	records, e := s.taskTimes(ctx, tx, owner, in, timezone)
	if e != nil {
		return e
	}
	zone, e := time.LoadLocation(timezone)
	if e != nil {
		return e
	}
	index := map[string]*Day{}
	for i := range days {
		days[i].TaskTimes = []TaskTime{}
		index[days[i].Date] = &days[i]
	}
	for _, v := range records {
		d := index[v.CompletedAt.In(zone).Format(time.DateOnly)]
		if d == nil {
			continue
		}
		d.TaskTimes = append(d.TaskTimes, v)
		if v.EstimatedMinutes != nil {
			d.EstimatedTasks++
		}
		if v.ActualSeconds != nil {
			d.ActualKnownTasks++
		}
		if v.EstimatedMinutes != nil && v.ActualSeconds != nil {
			d.ComparableTasks++
			d.ComparableEstimatedSeconds += int64(*v.EstimatedMinutes) * 60
			d.ComparableActualSeconds += *v.ActualSeconds
		}
	}
	return nil
}
func templateSamples(records []TaskTime) []TemplateSample {
	groups := map[string]*TemplateSample{}
	estimated := map[string]float64{}
	actual := map[string]float64{}
	for _, v := range records {
		if v.TemplateID == nil || v.TemplateVersion == nil || v.TemplateName == nil {
			continue
		}
		key := *v.TemplateID + "/" + strconv.FormatInt(*v.TemplateVersion, 10)
		g := groups[key]
		if g == nil {
			g = &TemplateSample{ID: *v.TemplateID, Name: *v.TemplateName, Version: *v.TemplateVersion, QuestIDs: []string{}}
			groups[key] = g
		}
		if v.EstimatedMinutes == nil || v.ActualSeconds == nil {
			continue
		}
		g.Count++
		g.QuestIDs = append(g.QuestIDs, v.ID)
		estimated[key] += float64(*v.EstimatedMinutes)
		actual[key] += float64(*v.ActualSeconds)
	}
	out := []TemplateSample{}
	for key, g := range groups {
		sort.Strings(g.QuestIDs)
		if g.Count > 0 {
			est, act := estimated[key]/float64(g.Count), actual[key]/float64(g.Count)
			g.AverageEstimatedMinutes = &est
			g.AverageActualSeconds = &act
			if est > 0 {
				factor := act / (60 * est)
				g.Factor = &factor
			}
		}
		g.Sufficient = g.Count >= 5
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Version < out[j].Version
	})
	return out
}
func (t *TimeTotals) add(v TimeTotals) {
	t.EstimatedTasks += v.EstimatedTasks
	t.ActualKnownTasks += v.ActualKnownTasks
	t.ComparableTasks += v.ComparableTasks
	t.ComparableEstimatedSeconds += v.ComparableEstimatedSeconds
	t.ComparableActualSeconds += v.ComparableActualSeconds
}

type TemplateSamples struct {
	Range
	Timezone   string           `json:"timezone"`
	ComputedAt time.Time        `json:"computed_at"`
	Items      []TemplateSample `json:"items"`
}

func (s *Service) mountTime(r chi.Router) {
	r.Get("/api/v1/analytics/templates", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.Read(r, func(ctx context.Context, tx pgx.Tx, owner string) (any, error) {
			in := Range{r.URL.Query().Get("from"), r.URL.Query().Get("to")}
			if _, _, e := validateRange(in); e != nil {
				return nil, e
			}
			var zone string
			if e := tx.QueryRow(ctx, `SELECT COALESCE(preferences->>'timezone','UTC') FROM user_settings WHERE owner_id=$1 FOR SHARE`, owner).Scan(&zone); e != nil {
				return nil, e
			}
			records, e := s.taskTimes(ctx, tx, owner, in, zone)
			if e != nil {
				return nil, e
			}
			return TemplateSamples{in, zone, s.Clock.Now(), templateSamples(records)}, nil
		})
		s.Reply(w, r, out, e)
	})
}
