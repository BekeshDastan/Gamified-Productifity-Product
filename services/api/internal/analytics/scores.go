package analytics

import (
	"math"
	"time"
)

const ScoreVersion = "score-v1"

type RollingScore struct {
	From             string   `json:"from"`
	To               string   `json:"to"`
	Score            *int     `json:"score"`
	ActiveDays       int      `json:"active_days"`
	TaskObligations  int      `json:"task_obligations"`
	HabitObligations int      `json:"habit_obligations"`
	Dates            []string `json:"dates"`
	State            string   `json:"state"`
}
type Scores struct {
	Version       string       `json:"version"`
	State         string       `json:"state"`
	Closed        bool         `json:"closed"`
	T             *float64     `json:"t"`
	H             *float64     `json:"h"`
	R             *float64     `json:"r"`
	Daily         *int         `json:"daily"`
	DisciplineDay *float64     `json:"discipline_day"`
	Discipline    RollingScore `json:"discipline"`
}

func ratio(v float64) *float64 { x := max(0, min(1, v)); return &x }
func weighted(t, h, r *float64, wt, wh, wr float64) *float64 {
	sum, weight := 0.0, 0.0
	for _, v := range []struct {
		x *float64
		w float64
	}{{t, wt}, {h, wh}, {r, wr}} {
		if v.x != nil {
			sum += *v.x * v.w
			weight += v.w
		}
	}
	if weight == 0 {
		return nil
	}
	out := 100 * sum / weight
	return &out
}

// scoreDay uses the original confirmation snapshot and frozen obligation periods.
func scoreDay(d Day, timezone string, now time.Time) Scores {
	out := Scores{Version: ScoreVersion, State: "no_plan"}
	zone, _ := time.LoadLocation(timezone)
	if zone == nil {
		out.State = "unavailable_timezone"
		return out
	}
	if d.Plan != nil && d.Plan.Confirmed {
		if z, e := time.LoadLocation(d.Plan.Timezone); e == nil {
			zone = z
		} else {
			out.State = "unavailable_timezone"
			return out
		}
	}
	day, e := time.ParseInLocation(time.DateOnly, d.Date, zone)
	if e != nil {
		out.State = "invalid_date"
		return out
	}
	out.Closed = !now.Before(day.AddDate(0, 0, 1))
	if d.Date > now.In(zone).Format(time.DateOnly) {
		out.State = "future"
		return out
	}
	if d.Plan != nil && d.Plan.Confirmed && d.Plan.Input.Rest {
		out.State = "rest"
		return out
	}
	if !d.HabitsComplete {
		out.State = "recalculating"
		return out
	}
	if d.Plan != nil && d.Plan.Confirmed && d.TaskWeightTotal > 0 {
		out.T = ratio(float64(d.TaskWeightCompleted) / float64(d.TaskWeightTotal))
	}
	sum, count := 0.0, 0
	for _, h := range d.Habits {
		if h.Excluded {
			continue
		}
		if !h.Closed {
			out.Closed = false
		}
		if h.Target > 0 {
			sum += *ratio(h.Amount / h.Target)
			count++
		}
	}
	if count > 0 {
		out.H = ratio(sum / float64(count))
	}
	if d.Plan != nil && d.Plan.Confirmed && d.Plan.Snapshot != nil {
		minutes := 0
		unknown := false
		for _, q := range d.Plan.Snapshot.Items {
			minutes += q.EstimatedMinutes
			unknown = unknown || q.EstimatedMinutes <= 0
		}
		if minutes > 0 && !unknown {
			out.R = ratio(float64(d.Plan.Input.AvailableMinutes) / float64(minutes))
		}
	}
	if out.T == nil && out.H == nil {
		return out
	}
	daily := weighted(out.T, out.H, nil, 0.6, 0.4, 0)
	if daily != nil {
		v := int(math.Round(*daily))
		out.Daily = &v
	}
	out.DisciplineDay = weighted(out.T, out.H, out.R, 0.50, 0.35, 0.15)
	out.State = "scored"
	if !out.Closed {
		out.State = "in_progress"
	}
	return out
}
func rollingScore(days []Day, date string, current Scores) RollingScore {
	end, _ := time.Parse(time.DateOnly, date)
	if current.Closed {
		end = end.AddDate(0, 0, 1)
	}
	from := end.AddDate(0, 0, -14)
	out := RollingScore{From: from.Format(time.DateOnly), To: end.Format(time.DateOnly), Dates: []string{}, State: "insufficient_data"}
	sum := 0.0
	if current.State == "future" {
		out.State = "future"
		return out
	}
	for _, d := range days {
		if d.Date < out.From || d.Date >= out.To || !d.Scores.Closed || d.Scores.DisciplineDay == nil {
			continue
		}
		out.ActiveDays++
		out.TaskObligations += d.Planned
		out.HabitObligations += d.HabitObligations
		out.Dates = append(out.Dates, d.Date)
		sum += *d.Scores.DisciplineDay
	}
	if out.ActiveDays >= 3 {
		v := int(math.Round(sum / float64(out.ActiveDays)))
		out.Score = &v
		out.State = "scored"
	}
	return out
}
