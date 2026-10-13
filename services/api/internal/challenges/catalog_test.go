package challenges

import "testing"

func TestCatalogHasTwelveChallengesAndTwentyOptionalQuests(t *testing.T) {
	catalog, e := ReadCatalog()
	if e != nil {
		t.Fatal(e)
	}
	if len(catalog.Challenges) != 12 || len(catalog.Quests) != 20 {
		t.Fatal("catalog count")
	}
	for _, template := range catalog.Challenges {
		if template.Days < 1 || template.Audience == "" || template.SafeNorm == "" {
			t.Fatal(template)
		}
	}
	for _, q := range catalog.Quests {
		if q.EstimatedMinutes > 45 || q.Target != 1 {
			t.Fatal(q)
		}
	}
}
func TestScoringUnitsAndBounds(t *testing.T) {
	catalog, e := ReadCatalog()
	if e != nil {
		t.Fatal(e)
	}
	base := catalog.Challenges[0].Config
	for _, change := range []func(*Config){func(v *Config) { v.Unit = "km" }, func(v *Config) { v.Metric = "unknown" }, func(v *Config) { v.Kind = "arbitrary" }, func(v *Config) { v.Days = 366 }, func(v *Config) { v.Target = 0 }, func(v *Config) { v.DailyCap = 241 }, func(v *Config) { v.AllowedMisses = 1 }, func(v *Config) { v.Period = "month" }} {
		v := base
		change(&v)
		if ValidateConfig(v) == nil {
			t.Fatal(v)
		}
	}
	periodic := catalog.Challenges[1].Config
	periodic.AllowedMisses = 0
	if ValidateConfig(periodic) == nil {
		t.Fatal("inconsistent misses")
	}
	periodic = catalog.Challenges[1].Config
	periodic.Target = 8
	if ValidateConfig(periodic) == nil {
		t.Fatal("too many periods")
	}
	if ValidateConfig(Config{Metric: "reviews", Unit: "reviews", Kind: "periodic", Period: "week", Target: 2, PeriodTarget: 7, Days: 8, DailyCap: 1}) == nil {
		t.Fatal("partial week cannot meet a full-week target")
	}
}
