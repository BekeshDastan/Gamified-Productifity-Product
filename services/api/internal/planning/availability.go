package planning

import (
	"context"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/jackc/pgx/v5"
	"sort"
	"time"
)

type Interval struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
type Conflict struct {
	First  string    `json:"first"`
	Second string    `json:"second"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
}
type Availability struct {
	WorkMinutes       int              `json:"work_minutes"`
	BusyMinutes       int              `json:"busy_minutes"`
	FreeMinutes       int              `json:"free_minutes"`
	ScheduledMinutes  int              `json:"scheduled_minutes"`
	RemainingCapacity int              `json:"remaining_capacity"`
	PlannedMinutes    int              `json:"planned_minutes"`
	RemainingMinutes  int              `json:"remaining_minutes"`
	DeficitMinutes    int              `json:"deficit_minutes"`
	FreeSlots         []Interval       `json:"free_slots"`
	Conflicts         []Conflict       `json:"conflicts"`
	MoreConflicts     bool             `json:"more_conflicts"`
	UnfitIDs          []string         `json:"unfit_ids"`
	UnknownIDs        []string         `json:"unknown_ids"`
	Blocks            []calendar.Event `json:"blocks"`
	Materializing     bool             `json:"materializing"`
}

func intersection(a, b Interval) (Interval, bool) {
	v := Interval{Start: a.Start, End: a.End}
	if b.Start.After(v.Start) {
		v.Start = b.Start
	}
	if b.End.Before(v.End) {
		v.End = b.End
	}
	return v, v.End.After(v.Start)
}
func union(in []Interval) []Interval {
	out := []Interval{}
	v := append([]Interval{}, in...)
	sort.Slice(v, func(i, j int) bool { return v[i].Start.Before(v[j].Start) })
	for _, x := range v {
		if !x.End.After(x.Start) {
			continue
		}
		if len(out) == 0 || x.Start.After(out[len(out)-1].End) {
			out = append(out, x)
		} else if x.End.After(out[len(out)-1].End) {
			out[len(out)-1].End = x.End
		}
	}
	return out
}
func subtract(windows, busy []Interval) []Interval {
	out := []Interval{}
	for _, w := range union(windows) {
		cursor := w.Start
		for _, b := range union(busy) {
			v, ok := intersection(w, b)
			if !ok {
				continue
			}
			if v.Start.After(cursor) {
				out = append(out, Interval{cursor, v.Start})
			}
			if v.End.After(cursor) {
				cursor = v.End
			}
		}
		if w.End.After(cursor) {
			out = append(out, Interval{cursor, w.End})
		}
	}
	return out
}
func minutes(in []Interval) int {
	total := time.Duration(0)
	for _, v := range union(in) {
		total += v.End.Sub(v.Start)
	}
	return int(total / time.Minute)
}
func windowsFor(date, zone string, in []Window) ([]Interval, error) {
	out := []Interval{}
	if len(in) == 0 {
		a, b, e := bounds(date, zone)
		return []Interval{{a, b}}, e
	}
	for _, w := range in {
		a, e := wall(date, zone, w.Start, false)
		if e != nil {
			return nil, e
		}
		b, e := wall(date, zone, w.End, true)
		if e != nil {
			return nil, e
		}
		if !b.After(a) {
			return nil, invalid("windows", "Окно должно иметь положительную длительность")
		}
		out = append(out, Interval{a, b})
	}
	return union(out), nil
}
func calculate(p Plan, windows []Interval, blocks []calendar.Event) (Availability, error) {
	out := Availability{FreeSlots: []Interval{}, Conflicts: []Conflict{}, UnfitIDs: []string{}, UnknownIDs: []string{}, Blocks: blocks}
	from, to, e := bounds(p.Date, p.Timezone)
	if e != nil {
		return out, e
	}
	day := Interval{from, to}
	busy := []Interval{}
	selected := map[string]bool{}
	for _, q := range p.QuestIDs {
		selected[q] = true
	}
	own := []Interval{}
	byTask := map[string][]Interval{}
	intervals := []Interval{}
	identities := []string{}
	for _, b := range blocks {
		a, e := time.Parse(time.RFC3339, b.Start)
		if e != nil {
			return out, e
		}
		end, e := time.Parse(time.RFC3339, b.End)
		if e != nil {
			return out, e
		}
		v, ok := intersection(Interval{a, end}, day)
		if !ok {
			continue
		}
		intervals = append(intervals, v)
		identities = append(identities, b.ID)
		busy = append(busy, v)
		if b.QuestID != nil && selected[*b.QuestID] {
			own = append(own, v)
			byTask[*b.QuestID] = append(byTask[*b.QuestID], v)
		}
	}
	// Keep conflict identities independent of unions; touching boundaries are not conflicts.
	for i := 0; i < len(intervals); i++ {
		for j := i + 1; j < len(intervals); j++ {
			v, ok := intersection(intervals[i], intervals[j])
			if !ok {
				continue
			}
			if len(out.Conflicts) == 200 {
				out.MoreConflicts = true
				continue
			}
			out.Conflicts = append(out.Conflicts, Conflict{identities[i], identities[j], v.Start, v.End})
		}
	}
	out.WorkMinutes = minutes(windows)
	out.FreeSlots = subtract(windows, busy)
	out.FreeMinutes = minutes(out.FreeSlots)
	out.BusyMinutes = out.WorkMinutes - out.FreeMinutes
	out.ScheduledMinutes = minutes(own)
	out.RemainingCapacity = max(0, min(p.AvailableMinutes-out.ScheduledMinutes, out.FreeMinutes))
	if p.Rest {
		out.RemainingCapacity = 0
	}
	items := map[string]Item{}
	for _, q := range p.Items {
		items[q.ID] = q
		out.PlannedMinutes += q.EstimatedMinutes
	}
	ordered := append([]string{}, p.TopIDs...)
	top := map[string]bool{}
	for _, id := range p.TopIDs {
		top[id] = true
	}
	for _, id := range p.QuestIDs {
		if !top[id] {
			ordered = append(ordered, id)
		}
	}
	remaining := out.RemainingCapacity
	for _, id := range ordered {
		q := items[id]
		if q.Archived || q.Status == "completed" || q.Status == "failed" || q.Status == "skipped" {
			continue
		}
		if q.EstimatedMinutes == 0 {
			out.UnknownIDs = append(out.UnknownIDs, id)
			continue
		}
		need := max(0, q.EstimatedMinutes-minutes(byTask[id]))
		out.RemainingMinutes += need
		if need > remaining {
			out.UnfitIDs = append(out.UnfitIDs, id)
		} else {
			remaining -= need
		}
	}
	out.DeficitMinutes = max(0, out.RemainingMinutes-out.RemainingCapacity)
	return out, nil
}
func (s *Service) view(ctx context.Context, tx pgx.Tx, id, date string) (Plan, error) {
	if e := s.lock(ctx, tx, id); e != nil {
		return Plan{}, e
	}
	d, e := day(date)
	if e != nil {
		return Plan{}, e
	}
	more, e := s.calendar.MaterializeInTx(ctx, tx, id, d, d.AddDate(0, 0, 1))
	if e != nil {
		return Plan{}, e
	}
	p, e := s.get(ctx, tx, id, date)
	if e != nil {
		return p, e
	}
	windows, e := windowsFor(date, p.Timezone, p.Windows)
	if e != nil {
		return p, e
	}
	from, to, e := bounds(date, p.Timezone)
	if e != nil {
		return p, e
	}
	blocks, e := s.calendar.TimedInTx(ctx, tx, id, from, to)
	if e != nil {
		return p, e
	}
	out, e := calculate(p, windows, blocks)
	out.Materializing = more
	p.Availability = &out
	return p, e
}
