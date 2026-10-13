package character

import "testing"

func TestEveryAchievementHasSuccessFailureAndAvailabilityFixture(t *testing.T) {
	defs := Definitions()
	if len(defs) != 30 {
		t.Fatal(len(defs))
	}
	seen := map[string]bool{}
	for _, d := range defs {
		t.Run(d.ID, func(t *testing.T) {
			if seen[d.ID] || d.Name == "" || d.Description == "" || d.Threshold < 1 || d.Version != AchievementVersion {
				t.Fatal(d)
			}
			seen[d.ID] = true
			f := Facts{Values: map[Metric]int64{d.Metric: d.Threshold - 1}, Available: map[Metric]bool{d.Metric: true}}
			if d.Met(f) {
				t.Fatal("below threshold")
			}
			f.Values[d.Metric]++
			if !d.Met(f) {
				t.Fatal("exact threshold")
			}
			f.Available[d.Metric] = false
			if d.Met(f) {
				t.Fatal("unavailable source must not award")
			}
		})
	}
}
