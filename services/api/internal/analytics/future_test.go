package analytics

import (
	"testing"
	"time"
)

func TestFutureSelfFourHoursTwelveWeeksZeroWeeksAndPartialObservation(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	created := now.AddDate(0, 0, -60)
	days := []Day{}
	end := fullWeekStart(now, time.UTC)
	for start := end.AddDate(0, 0, -28); start.Before(end); start = start.AddDate(0, 0, 7) {
		days = append(days, Day{Date: start.Format(time.DateOnly), FocusSeconds: 4 * 3600})
	}
	v, e := futureFromDays(now, created, "UTC", 12, days)
	if e != nil || !v.Available || v.ObservedWeeks != 4 || *v.AverageWeeklyHours != 4 || *v.ProjectedHours != 48 {
		t.Fatal(v, e)
	}
	days[0].FocusSeconds = 0
	v, e = futureFromDays(now, created, "UTC", 12, days)
	if e != nil || *v.AverageWeeklyHours != 3 || *v.ProjectedHours != 36 || *v.Weeks[0].FocusSeconds != 0 {
		t.Fatal(v, e)
	}
	v, e = futureFromDays(now, end.AddDate(0, 0, -8), "UTC", 12, days)
	if e != nil || v.Available || v.ObservedWeeks != 1 || v.ProjectedHours != nil || v.Weeks[2].FocusSeconds != nil {
		t.Fatal(v, e)
	}
	v, e = futureFromDays(now, end.AddDate(0, 0, -14), "UTC", 12, days)
	if e != nil || !v.Available || v.ObservedWeeks != 2 || *v.ProjectedHours != 48 {
		t.Fatal(v, e)
	}
	if _, e = futureFromDays(now, created, "UTC", 105, days); e == nil {
		t.Fatal("unbounded horizon")
	}
}
