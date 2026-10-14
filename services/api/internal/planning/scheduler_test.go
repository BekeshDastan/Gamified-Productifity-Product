package planning

import (
	"reflect"
	"testing"
	"time"
)

func TestSchedulerDeterministicOrderingDeadlinesAndNoPartialPlacements(t *testing.T) {
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	deadline := at.Add(2 * time.Hour)
	tasks := []ScheduleTask{{ID: "b", Minutes: 30, Priority: "high", CreatedAt: at}, {ID: "a", Minutes: 30, Priority: "high", CreatedAt: at}, {ID: "due", Minutes: 45, Priority: "low", CreatedAt: at, Deadline: &deadline}, {ID: "blocked", Minutes: 15, ExcludedReason: "blocked"}, {ID: "unknown"}}
	free := []Interval{{at, at.Add(time.Hour)}, {at.Add(90 * time.Minute), at.Add(2 * time.Hour)}}
	v := SuggestSlots(tasks, free, 5)
	again := SuggestSlots(tasks, free, 5)
	if !reflect.DeepEqual(v, again) {
		t.Fatal("unstable")
	}
	if len(v.Placements) != 2 || v.Placements[0].TaskID != "due" || v.Placements[1].TaskID != "a" {
		t.Fatal(v)
	}
	for _, p := range v.Placements {
		if p.Minutes < 15 || p.End.After(deadline) {
			t.Fatal(p)
		}
	}
	for i, a := range v.Placements {
		for _, b := range v.Placements[i+1:] {
			if a.Start.Before(b.End) && b.Start.Before(a.End) {
				t.Fatal("overlap")
			}
		}
	}
	if tasks[0].ID != "b" || !free[0].Start.Equal(at) {
		t.Fatal("input mutated")
	}
}
func TestSchedulerSplitsOnlyWithConsentAndMinimumChunk(t *testing.T) {
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	free := []Interval{{at, at.Add(20 * time.Minute)}, {at.Add(time.Hour), at.Add(80 * time.Minute)}}
	q := ScheduleTask{ID: "a", Minutes: 35}
	if v := SuggestSlots([]ScheduleTask{q}, free, 0); len(v.Placements) != 0 {
		t.Fatal(v)
	}
	q.AllowSplit = true
	v := SuggestSlots([]ScheduleTask{q}, free, 0)
	if len(v.Placements) != 2 || v.Placements[0].Minutes != 20 || v.Placements[1].Minutes != 15 {
		t.Fatal(v)
	}
	q.Minutes = 45
	v = SuggestSlots([]ScheduleTask{q}, free, 0)
	if len(v.Placements) != 0 || len(v.Unscheduled) != 1 || v.Unscheduled[0].DeficitMinutes != 5 {
		t.Fatal("partial task leaked", v)
	}
	q.FiveMinuteStart = true
	v = SuggestSlots([]ScheduleTask{q}, free, 0)
	if len(v.Placements) != 1 || v.Placements[0].Minutes != 5 || !v.Placements[0].PartialStart {
		t.Fatal(v)
	}
	q.FiveMinuteStart = false
	q.Minutes = 10
	v = SuggestSlots([]ScheduleTask{q}, free, 0)
	if len(v.Placements) != 0 || v.Unscheduled[0].Reason != "below_minimum" {
		t.Fatal(v)
	}
}
