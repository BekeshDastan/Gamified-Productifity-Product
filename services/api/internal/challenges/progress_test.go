package challenges

import (
	"testing"
	"time"
)

func TestScoringDailyCapsPeriodsAndMinutes(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	r := Rules{RuleInput: RuleInput{Config: Config{Metric: "tasks", Unit: "tasks", Kind: "cumulative", Target: 5, DailyCap: 2, Days: 7}, StartsOn: "2026-10-12", EndsOn: "2026-10-18"}, Timezone: "UTC"}
	p := calculate(r, []DayAmount{{"2026-10-12", 10}, {"2026-10-13", 1}}, 11, now)
	if p.Current != 3 || p.Percent != 60 || p.TargetMet || p.SourceCount != 11 {
		t.Fatal(p)
	}
	r.Kind = "periodic"
	r.Target = 4
	r.PeriodTarget = 2
	r.Period = "day"
	p = calculate(r, []DayAmount{{"2026-10-12", 10}, {"2026-10-14", 2}}, 12, now)
	if p.Current != 2 || p.Unit != "periods" || p.Periods[0].State != "met" || p.Periods[1].State != "missed" || p.Periods[3].State != "future" {
		t.Fatal(p)
	}
	r.Period = "week"
	r.Target = 1
	r.PeriodTarget = 5
	p = calculate(r, []DayAmount{{"2026-10-12", 10}, {"2026-10-13", 2}, {"2026-10-14", 2}}, 14, now)
	if p.Current != 1 || !p.TargetMet || len(p.Periods) != 1 || p.Percent != 100 {
		t.Fatal(p)
	}
	r.Kind = "cumulative"
	r.Unit = "minutes"
	r.Target = 10
	r.DailyCap = 5
	p = calculate(r, []DayAmount{{"2026-10-12", 400}, {"2026-10-13", 90}}, 2, now)
	if p.Current != 6.5 || p.Percent != 65 {
		t.Fatal(p)
	}
	p = calculate(r, nil, 0, now)
	if p.Current != 0 || p.Percent != 0 || p.TargetMet {
		t.Fatal(p)
	}
}
