package analytics

import (
	"context"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/jackc/pgx/v5"
	"time"
)

// OutcomeTotals use distinct entities. PostponementEvents is explicitly an event count.
type OutcomeTotals struct {
	Planned            int `json:"planned"`
	PlannedCompleted   int `json:"planned_completed"`
	PlannedFailed      int `json:"planned_failed"`
	PlannedSkipped     int `json:"planned_skipped"`
	PlannedPending     int `json:"planned_pending"`
	OnTime             int `json:"on_time"`
	Late               int `json:"late"`
	NoDeadline         int `json:"no_deadline"`
	Failed             int `json:"failed"`
	Skipped            int `json:"skipped"`
	Postponed          int `json:"postponed"`
	PostponementEvents int `json:"postponement_events"`
	HabitObligations   int `json:"habit_obligations"`
	HabitSuccesses     int `json:"habit_successes"`
	HabitMissed        int `json:"habit_missed"`
	HabitExcluded      int `json:"habit_excluded"`
	GoalsAchieved      int `json:"goals_achieved"`
	GoalsNotAchieved   int `json:"goals_not_achieved"`
	GoalsCancelled     int `json:"goals_cancelled"`
}
type PlanEvidence struct {
	Confirmed bool               `json:"confirmed"`
	Timezone  string             `json:"timezone"`
	Version   int64              `json:"version"`
	Input     planning.Input     `json:"input"`
	Snapshot  *planning.Snapshot `json:"snapshot"`
}
type HabitOutcome struct {
	ID       string    `json:"id"`
	PeriodID string    `json:"period_id"`
	Name     string    `json:"name"`
	Unit     string    `json:"unit"`
	URL      string    `json:"url"`
	Timezone string    `json:"timezone"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Amount   float64   `json:"amount"`
	Target   float64   `json:"target"`
	Excluded bool      `json:"excluded"`
	Closed   bool      `json:"closed"`
	AreaID   *string   `json:"area_id"`
}
type Outcomes struct {
	OutcomeTotals
	Plan                *PlanEvidence  `json:"plan"`
	TaskWeightTotal     int            `json:"task_weight_total"`
	TaskWeightCompleted int            `json:"task_weight_completed"`
	HabitsComplete      bool           `json:"habits_complete"`
	Habits              []HabitOutcome `json:"habits"`
}
type taskFact struct {
	ID, Title, Status   string
	Planned             *string
	Deadline, Completed *time.Time
	Version             int64
	AreaID              *string
	Leaf                bool
	UpdatedAt           time.Time
}

func (s *Service) outcomes(ctx context.Context, tx pgx.Tx, owner string, in Range, timezone string, days []Day, add func(Source)) error {
	index := map[string]*Day{}
	for i := range days {
		days[i].Habits = []HabitOutcome{}
		index[days[i].Date] = &days[i]
	}
	zone, e := time.LoadLocation(timezone)
	if e != nil {
		return e
	}
	tasks := map[string]taskFact{}
	scheduled := map[string][]string{}
	rows, e := tx.Query(ctx, `SELECT q.id::text,q.payload->>'title',q.status,q.planned_date::text,q.deadline,q.completed_at,q.version,q.area_id::text,q.updated_at,NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id) FROM quests q WHERE q.owner_id=$1 ORDER BY q.id`, owner)
	if e != nil {
		return e
	}
	for rows.Next() {
		var q taskFact
		if e = rows.Scan(&q.ID, &q.Title, &q.Status, &q.Planned, &q.Deadline, &q.Completed, &q.Version, &q.AreaID, &q.UpdatedAt, &q.Leaf); e != nil {
			rows.Close()
			return e
		}
		tasks[q.ID] = q
		if q.Leaf && q.Planned != nil {
			scheduled[*q.Planned] = append(scheduled[*q.Planned], q.ID)
		}
		if q.Leaf && q.Status == "completed" && q.Completed != nil {
			if d := index[q.Completed.In(zone).Format(time.DateOnly)]; d != nil {
				switch {
				case q.Deadline == nil:
					d.NoDeadline++
				case q.Completed.After(*q.Deadline):
					d.Late++
				default:
					d.OnTime++
				}
			}
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	rows, e = tx.Query(ctx, `SELECT local_date::text,timezone,version,payload,snapshot,confirmed_at FROM day_plans WHERE owner_id=$1 AND local_date>=$2::text::date AND local_date<$3::text::date ORDER BY local_date`, owner, in.From, in.To)
	if e != nil {
		return e
	}
	confirmTimes := map[string]time.Time{}
	for rows.Next() {
		var date string
		v := PlanEvidence{}
		var payload, snapshot []byte
		var confirmedAt *time.Time
		if e = rows.Scan(&date, &v.Timezone, &v.Version, &payload, &snapshot, &confirmedAt); e != nil {
			rows.Close()
			return e
		}
		if e = json.Unmarshal(payload, &v.Input); e != nil {
			rows.Close()
			return e
		}
		v.Confirmed = confirmedAt != nil
		if confirmedAt != nil {
			confirmTimes[date] = *confirmedAt
		}
		if snapshot != nil {
			v.Snapshot = &planning.Snapshot{}
			if e = json.Unmarshal(snapshot, v.Snapshot); e != nil {
				rows.Close()
				return e
			}
			v.Input = v.Snapshot.Input
		}
		index[date].Plan = &v
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for i := range days {
		d := &days[i]
		ids := scheduled[d.Date]
		weights := map[string]int{}
		at := s.Clock.Now()
		planZone := zone
		if d.Plan != nil {
			ids = d.Plan.Input.QuestIDs
			planZone, e = time.LoadLocation(d.Plan.Timezone)
			if e != nil {
				return e
			}
			if d.Plan.Confirmed {
				at = confirmTimes[d.Date]
				ids = []string{}
				for _, item := range d.Plan.Snapshot.Items {
					ids = append(ids, item.ID)
					weights[item.ID] = item.Weight
				}
			}
		}
		for _, id := range ids {
			q, ok := tasks[id]
			if !ok {
				continue
			}
			if d.Plan == nil || !d.Plan.Confirmed {
				if !q.Leaf {
					continue
				}
			}
			d.Planned++
			weight := weights[id]
			d.TaskWeightTotal += weight
			completedHere := q.Status == "completed" && q.Completed != nil && q.Completed.In(planZone).Format(time.DateOnly) == d.Date
			switch {
			case completedHere:
				d.PlannedCompleted++
				d.TaskWeightCompleted += weight
			case q.Status == "failed":
				d.PlannedFailed++
			case q.Status == "skipped":
				d.PlannedSkipped++
			default:
				d.PlannedPending++
			}
			if d.Plan == nil || !d.Plan.Confirmed {
				at = q.UpdatedAt
			}
			add(Source{Date: d.Date, Kind: "planned", ID: id, Name: q.Title, URL: "/app/quests/" + id, At: at, Value: 1, Unit: "tasks", AreaID: q.AreaID, Version: q.Version})
		}
	}
	// Postponements count distinct tasks per day; repeated postponements remain a separate count.
	rows, e = tx.Query(ctx, `SELECT quest_id::text,kind,created_at,quest_version FROM task_events WHERE owner_id=$1 AND kind IN('postpone','fail','skip') ORDER BY id`, owner)
	if e != nil {
		return e
	}
	postponed := map[string]bool{}
	latestTerminal := map[string]struct {
		kind    string
		at      time.Time
		version int64
	}{}
	for rows.Next() {
		var id, kind string
		var at time.Time
		var version int64
		if e = rows.Scan(&id, &kind, &at, &version); e != nil {
			rows.Close()
			return e
		}
		q := tasks[id]
		if !q.Leaf {
			continue
		}
		if kind != "postpone" {
			latestTerminal[id] = struct {
				kind    string
				at      time.Time
				version int64
			}{kind, at, version}
			continue
		}
		date := at.In(zone).Format(time.DateOnly)
		if d := index[date]; d != nil {
			d.PostponementEvents++
			key := date + "/" + id
			if !postponed[key] {
				d.Postponed++
				postponed[key] = true
			}
			add(Source{Date: date, Kind: "postponement", ID: id, Name: q.Title, URL: "/app/quests/" + id, At: at, Value: 1, Unit: "events", AreaID: q.AreaID, Version: version})
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for id, event := range latestTerminal {
		q := tasks[id]
		d := index[event.at.In(zone).Format(time.DateOnly)]
		if d == nil {
			continue
		}
		if event.kind == "fail" && q.Status == "failed" {
			d.Failed++
		} else if event.kind == "skip" && q.Status == "skipped" {
			d.Skipped++
		} else {
			continue
		}
		add(Source{Kind: event.kind, ID: id, Name: q.Title, URL: "/app/quests/" + id, At: event.at, Value: 1, Unit: "tasks", AreaID: q.AreaID, Version: event.version})
	}
	evidence, complete, e := habits.NewService(s.Pool, s.Clock).EvidenceInTx(ctx, tx, owner)
	if e != nil {
		return e
	}
	for i := range days {
		days[i].HabitsComplete = complete
	}
	units := map[string]string{}
	rows, e = tx.Query(ctx, `SELECT id::text,payload->>'unit' FROM habits WHERE owner_id=$1`, owner)
	if e != nil {
		return e
	}
	for rows.Next() {
		var id, unit string
		if e = rows.Scan(&id, &unit); e != nil {
			rows.Close()
			return e
		}
		units[id] = unit
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, h := range evidence {
		for _, p := range h.Periods {
			loc, e := time.LoadLocation(p.Timezone)
			if e != nil {
				return e
			}
			date := p.End.Add(-time.Nanosecond).In(loc).Format(time.DateOnly)
			d := index[date]
			if d == nil {
				continue
			}
			closed := !s.Clock.Now().Before(p.End)
			d.Habits = append(d.Habits, HabitOutcome{h.ID, p.ID, h.Title, units[h.ID], "/app/habits/" + h.ID, p.Timezone, p.Start, p.End, p.Amount, p.Target, p.Excluded, closed, h.AreaID})
			if p.Excluded {
				d.HabitExcluded++
				continue
			}
			d.HabitObligations++
			if p.Amount >= p.Target {
				d.HabitSuccesses++
			} else if closed {
				d.HabitMissed++
			}
			add(Source{Date: date, Kind: "habit", ID: p.ID, Name: h.Title, URL: "/app/habits/" + h.ID, At: p.End, Value: p.Amount, Unit: units[h.ID], AreaID: h.AreaID, Version: p.ScheduleVersion})
		}
	}
	// Declared goal quantities keep their own units and occurrence dates; diary text is never selected.
	rows, e = tx.Query(ctx, `SELECT e.id::text,e.goal_id::text,g.payload->>'title',g.area_id::text,e.value::float8,e.unit,e.occurred_on::text,e.created_at,e.version FROM goal_metric_entries e JOIN goals g ON g.owner_id=e.owner_id AND g.id=e.goal_id WHERE e.owner_id=$1 AND e.occurred_on>=$2::text::date AND e.occurred_on<$3::text::date ORDER BY e.id`, owner, in.From, in.To)
	if e != nil {
		return e
	}
	for rows.Next() {
		v := Source{Kind: "goal_metric"}
		var goalID string
		if e = rows.Scan(&v.ID, &goalID, &v.Name, &v.AreaID, &v.Value, &v.Unit, &v.Date, &v.At, &v.Version); e != nil {
			rows.Close()
			return e
		}
		v.URL = "/app/goals/" + goalID
		add(v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	rows, e = tx.Query(ctx, `SELECT g.id::text,g.payload->>'title',g.area_id::text,g.state,g.version,v.created_at FROM goals g JOIN LATERAL(SELECT created_at FROM (SELECT id,created_at,snapshot->>'state' state,lag(snapshot->>'state') OVER(ORDER BY id) previous_state FROM goal_versions WHERE owner_id=g.owner_id AND goal_id=g.id) transitions WHERE state=g.state AND previous_state IS DISTINCT FROM state ORDER BY id DESC LIMIT 1) v ON true WHERE g.owner_id=$1 AND g.state IN('achieved','not_achieved','cancelled') ORDER BY g.id`, owner)
	if e != nil {
		return e
	}
	for rows.Next() {
		v := Source{Kind: "goal_state", Value: 1, Unit: "goals"}
		var state string
		if e = rows.Scan(&v.ID, &v.Name, &v.AreaID, &state, &v.Version, &v.At); e != nil {
			rows.Close()
			return e
		}
		d := index[v.At.In(zone).Format(time.DateOnly)]
		if d == nil {
			continue
		}
		switch state {
		case "achieved":
			d.GoalsAchieved++
		case "not_achieved":
			d.GoalsNotAchieved++
		case "cancelled":
			d.GoalsCancelled++
		}
		v.Kind = "goal_" + state
		v.URL = "/app/goals/" + v.ID
		add(v)
	}
	e = rows.Err()
	rows.Close()
	return e
}
func (t *OutcomeTotals) add(d OutcomeTotals) {
	t.Planned += d.Planned
	t.PlannedCompleted += d.PlannedCompleted
	t.PlannedFailed += d.PlannedFailed
	t.PlannedSkipped += d.PlannedSkipped
	t.PlannedPending += d.PlannedPending
	t.OnTime += d.OnTime
	t.Late += d.Late
	t.NoDeadline += d.NoDeadline
	t.Failed += d.Failed
	t.Skipped += d.Skipped
	t.Postponed += d.Postponed
	t.PostponementEvents += d.PostponementEvents
	t.HabitObligations += d.HabitObligations
	t.HabitSuccesses += d.HabitSuccesses
	t.HabitMissed += d.HabitMissed
	t.HabitExcluded += d.HabitExcluded
	t.GoalsAchieved += d.GoalsAchieved
	t.GoalsNotAchieved += d.GoalsNotAchieved
	t.GoalsCancelled += d.GoalsCancelled
}
