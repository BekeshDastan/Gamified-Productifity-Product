package planning

import (
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"testing"
	"time"
)

func slot(a, b string) Interval {
	start, _ := time.Parse(time.RFC3339, "2026-10-12T"+a+":00Z")
	end, _ := time.Parse(time.RFC3339, "2026-10-12T"+b+":00Z")
	return Interval{start, end}
}
func timed(id, a, b string, q *string) calendar.Event {
	return calendar.Event{ID: id, Input: calendar.Input{Start: slot(a, b).Start.Format(time.RFC3339), End: slot(a, b).End.Format(time.RFC3339), QuestID: q}}
}
func TestAvailabilityUnionCapacityAndBoundary(t *testing.T) {
	q := "first"
	p := Plan{Date: "2026-10-12", Timezone: "UTC", Input: Input{AvailableMinutes: 120, QuestIDs: []string{"first", "second"}, TopIDs: []string{"first"}}, Items: []Item{{ID: "first", EstimatedMinutes: 90}, {ID: "second", EstimatedMinutes: 60}}}
	out, e := calculate(p, []Interval{slot("09:00", "12:00"), slot("11:00", "13:00")}, []calendar.Event{timed("a", "09:00", "10:00", &q), timed("b", "10:00", "10:30", nil), timed("c", "10:30", "11:00", nil), timed("d", "10:15", "10:45", nil)})
	if e != nil || out.WorkMinutes != 240 || out.BusyMinutes != 120 || out.FreeMinutes != 120 || out.RemainingCapacity != 60 || out.RemainingMinutes != 90 || out.DeficitMinutes != 30 || len(out.UnfitIDs) != 1 || out.UnfitIDs[0] != "second" || len(out.Conflicts) != 2 {
		t.Fatal(out, e)
	}
	out, e = calculate(p, []Interval{slot("09:00", "12:00")}, []calendar.Event{timed("a", "09:00", "10:00", &q), timed("b", "10:00", "11:00", nil)})
	if e != nil || len(out.Conflicts) != 0 {
		t.Fatal("touching boundaries conflict", out, e)
	}
}
func TestAvailabilityDSTWindowsAndUnknownEstimates(t *testing.T) {
	for _, tc := range []struct {
		date    string
		minutes int
	}{{"2026-03-08", 120}, {"2026-11-01", 240}} {
		w, e := windowsFor(tc.date, "America/New_York", []Window{{Start: "01:00", End: "04:00"}})
		if e != nil || minutes(w) != tc.minutes {
			t.Fatal(tc, minutes(w), e)
		}
	}
	if _, e := windowsFor("2026-03-08", "America/New_York", []Window{{Start: "02:30", End: "04:00"}}); e == nil {
		t.Fatal("missing DST time accepted")
	}
	p := Plan{Date: "2026-10-12", Timezone: "UTC", Input: Input{AvailableMinutes: 60, QuestIDs: []string{"unknown", "done"}}, Items: []Item{{ID: "unknown"}, {ID: "done", EstimatedMinutes: 90, Status: "completed"}}}
	out, e := calculate(p, []Interval{slot("09:00", "10:00")}, []calendar.Event{})
	if e != nil || len(out.UnknownIDs) != 1 || out.DeficitMinutes != 0 {
		t.Fatal(out, e)
	}
}
