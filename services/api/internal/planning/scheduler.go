package planning

import (
	"sort"
	"time"
)

type ScheduleTask struct {
	ID              string     `json:"id"`
	Version         int64      `json:"version"`
	Title           string     `json:"title"`
	Minutes         int        `json:"minutes"`
	Priority        string     `json:"priority"`
	CreatedAt       time.Time  `json:"created_at"`
	Deadline        *time.Time `json:"deadline"`
	AllowSplit      bool       `json:"allow_split"`
	FiveMinuteStart bool       `json:"five_minute_start"`
	ExcludedReason  string     `json:"excluded_reason"`
}
type Placement struct {
	TaskID       string    `json:"task_id"`
	TaskVersion  int64     `json:"task_version"`
	Title        string    `json:"title"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	BreakEnd     time.Time `json:"break_end"`
	Minutes      int       `json:"minutes"`
	PartialStart bool      `json:"partial_start"`
}
type Unscheduled struct {
	TaskID          string `json:"task_id"`
	Reason          string `json:"reason"`
	RequiredMinutes int    `json:"required_minutes"`
	FreeMinutes     int    `json:"free_minutes"`
	DeficitMinutes  int    `json:"deficit_minutes"`
}
type Suggestion struct {
	Placements  []Placement   `json:"placements"`
	Unscheduled []Unscheduled `json:"unscheduled"`
	Warnings    []string      `json:"warnings"`
}

func priority(value string) int { return map[string]int{"high": 3, "normal": 2, "low": 1}[value] }

// SuggestSlots operates on copies. A task is committed to the suggestion only if
// its complete estimate fits, except an explicitly requested five-minute start.
func SuggestSlots(tasks []ScheduleTask, free []Interval, breakMinutes int) Suggestion {
	breakMinutes = max(0, min(60, breakMinutes))
	out := Suggestion{[]Placement{}, []Unscheduled{}, []string{}}
	ordered := append([]ScheduleTask{}, tasks...)
	slots := union(free)
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if (a.Deadline == nil) != (b.Deadline == nil) {
			return a.Deadline != nil
		}
		if a.Deadline != nil && !a.Deadline.Equal(*b.Deadline) {
			return a.Deadline.Before(*b.Deadline)
		}
		if priority(a.Priority) != priority(b.Priority) {
			return priority(a.Priority) > priority(b.Priority)
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	})
	for _, q := range ordered {
		need := q.Minutes
		minimum := 15
		if q.FiveMinuteStart {
			need = 5
			minimum = 5
		}
		reason := q.ExcludedReason
		if reason == "" && q.Minutes <= 0 {
			reason = "unknown_estimate"
		}
		if reason == "" && need < minimum {
			reason = "below_minimum"
		}
		possible := 0
		for _, v := range slots {
			end := v.End
			if q.Deadline != nil && q.Deadline.Before(end) {
				end = *q.Deadline
			}
			if end.After(v.Start) {
				possible += int(end.Sub(v.Start) / time.Minute)
			}
		}
		if reason != "" {
			out.Unscheduled = append(out.Unscheduled, Unscheduled{q.ID, reason, need, possible, max(0, need-possible)})
			continue
		}
		working := append([]Interval{}, slots...)
		pieces := []Placement{}
		remaining := need
		for i, v := range working {
			if remaining == 0 {
				break
			}
			end := v.End
			if q.Deadline != nil && q.Deadline.Before(end) {
				end = *q.Deadline
			}
			available := int(end.Sub(v.Start) / time.Minute)
			if available < minimum {
				continue
			}
			chunk := remaining
			if chunk > available {
				if !q.AllowSplit || q.FiveMinuteStart {
					continue
				}
				chunk = available
				if remaining-chunk < minimum {
					chunk -= minimum - (remaining - chunk)
				}
			}
			if chunk < minimum {
				continue
			}
			finish := v.Start.Add(time.Duration(chunk) * time.Minute)
			breakEnd := finish.Add(time.Duration(breakMinutes) * time.Minute)
			if breakEnd.After(v.End) {
				breakEnd = v.End
			}
			working[i].Start = breakEnd
			remaining -= chunk
			pieces = append(pieces, Placement{q.ID, q.Version, q.Title, v.Start, finish, breakEnd, chunk, q.FiveMinuteStart})
		}
		if remaining > 0 {
			out.Unscheduled = append(out.Unscheduled, Unscheduled{q.ID, "capacity", need, possible, max(0, need-possible)})
			continue
		}
		slots = working
		out.Placements = append(out.Placements, pieces...)
	}
	if len(out.Unscheduled) > 0 {
		out.Warnings = append(out.Warnings, "Часть задач не размещена. Свободные минуты не гарантируют цельный подходящий слот.")
	}
	return out
}
