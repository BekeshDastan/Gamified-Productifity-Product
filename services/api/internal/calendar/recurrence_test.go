package calendar

import (
	"reflect"
	"testing"
	"time"
)

func mustDay(t *testing.T, v string) time.Time {
	t.Helper()
	d, e := day(v)
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func TestRecurrenceCalendarBoundaries(t *testing.T) {
	cases := []struct {
		name, start, end, freq string
		interval               int
		weekdays               []int
		count                  int
		want                   []string
	}{
		{"31 skips short months", "2028-01-31", "2028-02-01", "monthly", 1, nil, 3, []string{"2028-01-31", "2028-03-31", "2028-05-31"}},
		{"leap day", "2028-02-28", "2028-02-29", "daily", 1, nil, 3, []string{"2028-02-28", "2028-02-29", "2028-03-01"}},
		{"year boundary", "2026-12-31", "2027-01-01", "daily", 1, nil, 2, []string{"2026-12-31", "2027-01-01"}},
		{"every two weeks Monday Wednesday", "2026-10-12", "2026-10-13", "weekly", 2, []int{1, 3}, 4, []string{"2026-10-12", "2026-10-14", "2026-10-26", "2026-10-28"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, zone := range []string{"UTC", "Asia/Almaty", "America/New_York"} {
				in := SeriesInput{Event: Input{Title: "Проверка", AllDay: true, Start: tc.start, End: tc.end, Timezone: zone}, Kind: "event", Rule: Rule{Frequency: tc.freq, Interval: tc.interval, Weekdays: tc.weekdays, Count: tc.count}}
				out, e := Generate(in, mustDay(t, tc.start), mustDay(t, "2029-01-01"), nil)
				if e != nil {
					t.Fatal(e)
				}
				got := []string{}
				for _, v := range out {
					got = append(got, v.Key)
					if v.Event.Start != v.Key {
						t.Fatal("all day shifted", zone)
					}
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatal(zone, got, tc.want)
				}
			}
		})
	}
}
func TestRecurrenceDSTGapFoldAndRealDuration(t *testing.T) {
	ny, e := time.LoadLocation("America/New_York")
	if e != nil {
		t.Fatal(e)
	}
	if _, ok := ResolveWall(mustDay(t, "2026-03-08"), 2, 30, ny); ok {
		t.Fatal("missing hour accepted")
	}
	fold, ok := ResolveWall(mustDay(t, "2026-11-01"), 1, 30, ny)
	if !ok || fold.Format(time.RFC3339) != "2026-11-01T05:30:00Z" {
		t.Fatal("not earlier fold", fold)
	}
	in := SeriesInput{Event: Input{Title: "Часы", Start: "2026-03-07T02:30:00-05:00", End: "2026-03-07T03:30:00-05:00", Timezone: "America/New_York"}, Kind: "event", Rule: Rule{Frequency: "daily", Interval: 1, Count: 3}}
	out, e := Generate(in, mustDay(t, "2026-03-07"), mustDay(t, "2026-03-12"), nil)
	if e != nil {
		t.Fatal(e)
	}
	want := []string{"2026-03-07", "2026-03-09", "2026-03-10"}
	got := []string{}
	for _, v := range out {
		got = append(got, v.Key)
		a, _ := time.Parse(time.RFC3339, v.Event.Start)
		b, _ := time.Parse(time.RFC3339, v.Event.End)
		if b.Sub(a) != time.Hour || a.In(ny).Hour() != 2 {
			t.Fatal("duration or wall hour changed")
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	lordHowe, e := time.LoadLocation("Australia/Lord_Howe")
	if e != nil {
		t.Fatal(e)
	}
	if _, ok := ResolveWall(mustDay(t, "2026-10-04"), 2, 15, lordHowe); ok {
		t.Fatal("half hour gap accepted")
	}
	in.Rule.Count = 10000
	if _, e = Generate(in, mustDay(t, "2026-03-07"), mustDay(t, "2026-03-12"), nil); e == nil {
		t.Fatal("unreachable count silently truncated")
	}
}
