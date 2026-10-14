//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"testing"
	"time"
)

func TestNavigatorStructuredHabitGoalCapacityTemplateConflictAndFocusFacts(t *testing.T) {
	limits := auth.DefaultLimits()
	limits.UserRequests = 1000
	f := newAuthFixture(t, limits)
	a, who := f.signedIn(t, "navigator-facts@example.test")
	hInput := habitInput()
	hInput.Schedule.Weekdays = []int{1, 2, 3, 4, 5, 6, 7}
	hInput.Schedule.Target = 1
	hInput.Title = "Обязательные периоды"
	readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", hInput))
	hInput.Title = "Последовательные периоды"
	good := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", hInput))
	for i := 0; i < 7; i++ {
		f.loginAs(t, a, "navigator-facts@example.test")
		good = readHabit(t, f.privateRequest(t, a, "GET", "/habits/"+good.ID, "", nil))
		var period string
		for _, p := range good.Periods {
			if !f.clock.Now().Before(p.Start) && f.clock.Now().Before(p.End) {
				period = p.ID
			}
		}
		if period == "" {
			t.Fatal("missing current period")
		}
		readHabit(t, f.privateRequest(t, a, "POST", "/habits/"+good.ID+"/logs", habitVersion(good), habits.LogInput{PeriodID: period, Amount: 1, OccurredAt: f.clock.Now()}))
		f.clock.Advance(24 * time.Hour)
	}
	f.loginAs(t, a, "navigator-facts@example.test")
	gInput := goalInput("Числовая цель")
	gInput.ProgressType = "metric"
	gInput.Unit = "страниц"
	gInput.Target = 100
	past := "2026-10-13"
	gInput.TargetDate = &past
	g := readGoal(t, f.privateRequest(t, a, "POST", "/goals", "", goals.Create{Input: gInput, State: "active"}))
	readGoal(t, f.privateRequest(t, a, "POST", "/goals/"+g.ID+"/entries", goalVersion(g), goals.MetricInput{Value: 50, Unit: "страниц", OccurredOn: "2026-10-19", Note: "never exposed"}))
	qInput := questInput("Срочный шаг")
	qInput.EstimatedMinutes = 90
	deadline := f.clock.Now().Add(2 * time.Hour)
	qInput.Deadline = &deadline
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", qInput))
	readPlan(t, f.privateRequest(t, a, "PUT", "/plans/day/2026-10-19", `"0"`, planning.Input{AvailableMinutes: 30, Windows: []planning.Window{{Start: "13:00", End: "14:00"}}, QuestIDs: []string{q.ID}}))
	for _, v := range []struct{ start, end string }{{"13:10", "13:30"}, {"13:20", "13:40"}} {
		readEvent(t, f.privateRequest(t, a, "POST", "/calendar/events", "", calendar.Input{Title: "Фиксированный блок", Start: "2026-10-19T" + v.start + ":00Z", End: "2026-10-19T" + v.end + ":00Z", Timezone: "UTC"}))
	}
	seedInput := questInput("Образец")
	seedInput.EstimatedMinutes = 10
	seed := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", seedInput))
	r := f.privateRequest(t, a, "POST", "/templates/quests", "", quests.TemplateSource{Name: "Повторяемый шаг", QuestID: seed.ID})
	var template struct{ Data quests.Template }
	if r.status != 200 || json.Unmarshal(r.body, &template) != nil {
		t.Fatal(r.status, string(r.body))
	}
	for i := 0; i < 5; i++ {
		task := readQuest(t, f.privateRequest(t, a, "POST", "/templates/quests/"+template.Data.ID+"/apply", `"1"`, quests.TemplateDates{}))
		edit := task.Input
		edit.ActualTimeMode = "manual_override"
		edit.ActualMinutes = 20
		task = readQuest(t, f.privateRequest(t, a, "PATCH", "/quests/"+task.ID, version(task), edit))
		readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+task.ID+"/complete", version(task), quests.Transition{}))
	}
	session := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", focusInput()))
	f.clock.Advance(time.Minute)
	focusAct(t, f, a, session, "finish")
	for _, id := range []string{"R02", "R03", "R04", "R09", "R10", "R11", "R12", "R13", "R15"} {
		if _, e := f.pool.Exec(f.ctx, `INSERT INTO navigator_rule_preferences(owner_id,rule_id,disabled) SELECT $1,id,id<>$2 FROM navigator_rule_versions ON CONFLICT(owner_id,rule_id) DO UPDATE SET disabled=excluded.disabled`, who.ID, id); e != nil {
			t.Fatal(e)
		}
		page := readNavigator(t, f.privateRequest(t, a, "GET", "/navigator", "", nil))
		if len(page.Items) == 0 || page.Items[0].Rule.ID != id || len(page.Items[0].Fact.Sources) == 0 {
			t.Fatal("missing structured fact", id, page)
		}
	}
	f.clock.Advance(7 * 24 * time.Hour)
	f.loginAs(t, a, "navigator-facts@example.test")
	f.pool.Exec(f.ctx, `UPDATE navigator_rule_preferences SET disabled=(rule_id<>'R08') WHERE owner_id=$1`, who.ID)
	page := readNavigator(t, f.privateRequest(t, a, "GET", "/navigator", "", nil))
	if len(page.Items) == 0 || page.Items[0].Rule.ID != "R08" {
		t.Fatal("missing closed-week review", page)
	}
}
