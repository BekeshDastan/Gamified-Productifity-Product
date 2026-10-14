package analytics

import (
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"testing"
	"time"
)

func scoreFixture() Day {
	return Day{Date: "2026-10-12", Outcomes: Outcomes{Plan: &PlanEvidence{Confirmed: true, Timezone: "UTC", Input: planning.Input{AvailableMinutes: 100}, Snapshot: &planning.Snapshot{Items: []planning.Item{{Weight: 3, EstimatedMinutes: 75}, {Weight: 1, EstimatedMinutes: 25}}}}, TaskWeightTotal: 4, TaskWeightCompleted: 3, HabitsComplete: true, Habits: []HabitOutcome{{Amount: 10, Target: 20, Closed: true}}, OutcomeTotals: OutcomeTotals{Planned: 2, HabitObligations: 1}}}
}
func TestScoresDaily65AndVersionedDiscipline70(t *testing.T) {
	d := scoreFixture()
	s := scoreDay(d, "UTC", time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC))
	if s.Version != ScoreVersion || s.Daily == nil || *s.Daily != 65 || s.T == nil || *s.T != 0.75 || s.H == nil || *s.H != 0.5 || s.R == nil || *s.R != 1 || s.DisciplineDay == nil || *s.DisciplineDay < 69.999999 || *s.DisciplineDay > 70.000001 || !s.Closed || s.State != "scored" {
		t.Fatal(s)
	}
}
func TestScoresNullComponentsRestUnknownAndFrozenZone(t *testing.T) {
	now := time.Date(2026, 10, 13, 1, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		edit     func(*Day)
		timezone string
		daily    *int
		state    string
		closed   bool
	}{
		{"tasks only", func(d *Day) { d.Habits = nil }, "UTC", scoreInt(75), "scored", true},
		{"habit only", func(d *Day) { d.Plan = nil; d.TaskWeightTotal = 0 }, "UTC", scoreInt(50), "scored", true},
		{"rest", func(d *Day) { d.Plan.Input.Rest = true }, "UTC", nil, "rest", true},
		{"empty", func(d *Day) { d.Plan = nil; d.Habits = nil }, "UTC", nil, "no_plan", true},
		{"unconfirmed", func(d *Day) { d.Plan.Confirmed = false; d.Habits = nil }, "UTC", nil, "no_plan", true},
		{"missed obligations", func(d *Day) { d.TaskWeightCompleted = 0; d.Habits[0].Amount = 0 }, "UTC", scoreInt(0), "scored", true},
		{"clamped overachievement", func(d *Day) { d.TaskWeightCompleted = 8; d.Habits[0].Amount = 100 }, "UTC", scoreInt(100), "scored", true},
		{"excluded habit", func(d *Day) { d.Habits[0].Excluded = true }, "UTC", scoreInt(75), "scored", true},
		{"zero target", func(d *Day) { d.Habits[0].Target = 0 }, "UTC", scoreInt(75), "scored", true},
		{"materializing", func(d *Day) { d.HabitsComplete = false }, "UTC", nil, "recalculating", true},
		{"future", func(d *Day) { d.Date = "2026-10-14" }, "UTC", nil, "future", false},
		{"current day", func(d *Day) { d.Date = "2026-10-13"; d.Habits[0].Closed = false }, "UTC", scoreInt(65), "in_progress", false},
		{"period still open", func(d *Day) { d.Habits[0].Closed = false }, "UTC", scoreInt(65), "in_progress", false},
		{"frozen Honolulu day", func(d *Day) { d.Plan.Timezone = "Pacific/Honolulu" }, "Asia/Almaty", scoreInt(65), "in_progress", false},
		{"invalid zone", func(d *Day) { d.Plan = nil }, "Invalid/Zone", nil, "unavailable_timezone", false},
		{"invalid frozen zone", func(d *Day) { d.Plan.Timezone = "Invalid/Zone" }, "UTC", nil, "unavailable_timezone", false},
		{"invalid date", func(d *Day) { d.Date = "bad" }, "UTC", nil, "invalid_date", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := scoreFixture()
			tc.edit(&d)
			s := scoreDay(d, tc.timezone, now)
			if s.State != tc.state || s.Closed != tc.closed || (s.Daily == nil) != (tc.daily == nil) || (s.Daily != nil && *s.Daily != *tc.daily) {
				t.Fatal(s)
			}
		})
	}
	d := scoreFixture()
	d.Plan.Snapshot.Items[0].EstimatedMinutes = 0
	if s := scoreDay(d, "UTC", now); s.R != nil || s.Daily == nil || *s.Daily != 65 {
		t.Fatal("unknown time pretends known feasibility", s)
	}
	d = scoreFixture()
	d.Plan.Input.AvailableMinutes = 50
	if s := scoreDay(d, "UTC", now); s.R == nil || *s.R != 0.5 {
		t.Fatal(s)
	}
}
func scoreInt(v int) *int { return &v }
func TestScoresRollingRequiresThreeClosedDaysKeepsZeroAndBounds(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	days := []Day{}
	for _, date := range []string{"2026-09-29", "2026-10-01", "2026-10-12", "2026-10-13", "2026-10-14", "2026-10-15"} {
		d := scoreFixture()
		d.Date = date
		if date == "2026-10-13" {
			d.TaskWeightCompleted = 0
			d.Habits[0].Amount = 0
			d.Plan.Input.AvailableMinutes = 0
		}
		d.Scores = scoreDay(d, "UTC", now)
		days = append(days, d)
	}
	current := scoreDay(scoreFixture(), "UTC", now)
	current.Closed = false
	r := rollingScore(days, "2026-10-15", current)
	if r.From != "2026-10-01" || r.To != "2026-10-15" || r.ActiveDays != 4 || r.Score == nil || *r.Score != 53 || r.TaskObligations != 8 || len(r.Dates) != 4 {
		t.Fatal("zero day excluded or wrong calendar window", r)
	}
	fewer := rollingScore(days[2:4], "2026-10-15", current)
	if fewer.Score != nil || fewer.ActiveDays != 2 || fewer.State != "insufficient_data" {
		t.Fatal(fewer)
	}
	current.State = "future"
	if v := rollingScore(days, "2026-10-15", current); v.Score != nil || v.State != "future" {
		t.Fatal(v)
	}
	empty := weighted(nil, nil, nil, .5, .35, .15)
	if empty != nil {
		t.Fatal("empty weighted score")
	}
}
