package habits

import (
	"math"
	"testing"
	"time"
)

func TestHabitValidation(t *testing.T) {
	base := Input{Title: "Чтение", Kind: "count", Unit: "страниц", Minimum: 1, StartsOn: "2026-10-12", Schedule: Schedule{Frequency: "daily", Target: 20, Weekdays: []int{1, 3, 5}}}
	if _, e := Validate(base); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*Input){func(v *Input) { v.Title = "" }, func(v *Input) { v.Kind = "bad" }, func(v *Input) { v.Unit = "" }, func(v *Input) { v.Minimum = 0 }, func(v *Input) { v.Minimum = 21 }, func(v *Input) { v.Schedule.Target = 0 }, func(v *Input) { v.Schedule.Target = math.Inf(1) }, func(v *Input) { v.Schedule.Frequency = "year" }, func(v *Input) { v.Schedule.Weekdays = []int{1, 1} }, func(v *Input) { v.Schedule.Weekdays = nil }, func(v *Input) { v.Kind = "binary" }, func(v *Input) { v.Kind = "duration" }, func(v *Input) { v.StartsOn = "bad" }, func(v *Input) { s := "2026-10-11"; v.EndsOn = &s }, func(v *Input) { s := "bad"; v.AreaID = &s }} {
		v := base
		change(&v)
		if _, e := Validate(v); e == nil {
			t.Fatal("invalid habit accepted", v)
		}
	}
	for _, kind := range []string{"binary", "duration", "count"} {
		v := base
		v.Kind = kind
		if kind == "binary" {
			v.Schedule.Target = 1
		}
		if kind == "duration" {
			v.Unit = "мин"
		}
		if _, e := Validate(v); e != nil {
			t.Fatal(kind, e)
		}
	}
}
func TestPeriodBoundsAndWeekdays(t *testing.T) {
	z, _ := time.LoadLocation("America/New_York")
	for _, tc := range []struct {
		date, frequency, start, end string
		hours                       int
	}{{"2026-03-08", "daily", "2026-03-08", "2026-03-09", 23}, {"2026-11-01", "daily", "2026-11-01", "2026-11-02", 25}, {"2026-10-14", "weekly", "2026-10-12", "2026-10-19", 168}, {"2026-12-31", "monthly", "2026-12-01", "2027-01-01", 744}, {"2028-02-29", "monthly", "2028-02-01", "2028-03-01", 696}} {
		d, _ := date(tc.date)
		a, b := periodBounds(d, tc.frequency, z)
		if a.Format(time.DateOnly) != tc.start || b.Format(time.DateOnly) != tc.end || int(b.Sub(a).Hours()) != tc.hours {
			t.Fatal(tc, a, b)
		}
	}
	mon, _ := date("2026-10-12")
	tue := mon.AddDate(0, 0, 1)
	s := Schedule{Frequency: "daily", Weekdays: []int{1, 3, 5}}
	if !scheduled(mon, s) || scheduled(tue, s) {
		t.Fatal("unscheduled day counted")
	}
}
