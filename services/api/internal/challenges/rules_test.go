package challenges

import (
	"testing"
	"time"
)

func TestFrozenLocalDateBoundsRespectDST(t *testing.T) {
	for _, test := range []struct {
		start, end, zone string
		hours            int
	}{{"2026-11-01", "2026-11-02", "America/New_York", 49}, {"2026-03-08", "2026-03-09", "America/New_York", 47}, {"2026-10-12", "2026-10-13", "Asia/Almaty", 48}, {"2026-10-12", "2026-10-13", "UTC", 48}} {
		catalog, e := ReadCatalog()
		if e != nil {
			t.Fatal(e)
		}
		cfg := catalog.Challenges[0].Config
		cfg.Days = 2
		now, _ := time.Parse("2006-01-02", test.start)
		now = now.Add(12 * time.Hour)
		rules, from, to, e := validateRules(RuleInput{Config: cfg, Name: "Мой ритм", StartsOn: test.start, EndsOn: test.end}, test.zone, now)
		if e != nil || int(to.Sub(from)/time.Hour) != test.hours || rules.Timezone != test.zone || rules.Visibility != "private" {
			t.Fatal(test, rules, from, to, e)
		}
	}
}
func TestRulesRejectPastDatesAndMismatchedSourceKinds(t *testing.T) {
	catalog, _ := ReadCatalog()
	input := RuleInput{Config: catalog.Challenges[0].Config, Name: "Фокус", StartsOn: "2026-10-12", EndsOn: "2026-10-18"}
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	for _, change := range []func(*RuleInput){func(v *RuleInput) { v.StartsOn = "2026-10-11" }, func(v *RuleInput) { v.EndsOn = "2026-10-19" }, func(v *RuleInput) { v.Name = "a\nb" }, func(v *RuleInput) { id := "bad"; v.GoalID = &id }, func(v *RuleInput) { v.Metric = "habits"; v.Unit = "periods"; v.DailyCap = 10; v.Target = 3 }} {
		v := input
		change(&v)
		if _, _, _, e := validateRules(v, "UTC", now); e == nil {
			t.Fatal(v)
		}
	}
}
