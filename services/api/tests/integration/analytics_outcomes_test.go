//go:build integration

package integration

import (
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"strings"
	"testing"
	"time"
)

func TestAnalyticsOutcomeDenominatorsAndGoalUnits(t *testing.T) {
	limits := auth.DefaultLimits()
	limits.UserRequests = 600
	f := newAuthFixture(t, limits)
	a, _ := f.signedIn(t, "analytics-outcomes@example.test")
	date := "2026-10-12"
	tasks := []quests.Quest{}
	for i, title := range []string{"Вовремя", "Поздно", "Без срока", "Провал", "Пропуск", "Перенос", "Ожидает"} {
		in := questInput(title)
		in.PlannedDate = &date
		in.EstimatedMinutes = 10
		if i < 2 {
			at := f.clock.Now().Add(time.Hour)
			if i == 1 {
				at = f.clock.Now().Add(-time.Hour)
			}
			in.Deadline = &at
		}
		tasks = append(tasks, readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", in)))
	}
	ids := []string{}
	for _, q := range tasks {
		ids = append(ids, q.ID)
	}
	p := readPlan(t, f.privateRequest(t, a, "PUT", "/plans/day/"+date, `"0"`, planning.Input{AvailableMinutes: 120, QuestIDs: ids}))
	readPlan(t, f.privateRequest(t, a, "POST", "/plans/day/"+date+"/confirm", planVersion(p), struct{}{}))
	for i, action := range []string{"complete", "complete", "complete", "fail", "skip"} {
		q := tasks[i]
		readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/"+action, version(q), quests.Transition{Reason: "Зафиксирован реальный результат"}))
	}
	q := tasks[5]
	next := "2026-10-13"
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/postpone", version(q), quests.Transition{Reason: "Переношу", PlannedDate: &next}))
	next = "2026-10-14"
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/postpone", version(q), quests.Transition{Reason: "Пересмотр даты", PlannedDate: &next}))
	for _, amount := range []float64{10, 20} {
		h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", habitInput()))
		readHabit(t, f.privateRequest(t, a, "POST", "/habits/"+h.ID+"/logs", habitVersion(h), habits.LogInput{PeriodID: h.Periods[0].ID, Amount: amount, OccurredAt: f.clock.Now()}))
	}
	input := goalInput("Чтение по страницам")
	input.ProgressType = "metric"
	input.Target = 20
	input.Unit = "страниц"
	g := readGoal(t, f.privateRequest(t, a, "POST", "/goals", "", goals.Create{Input: input, State: "active"}))
	g = readGoal(t, f.privateRequest(t, a, "POST", "/goals/"+g.ID+"/entries", goalVersion(g), goals.MetricInput{Value: 20, Unit: "страниц", OccurredOn: date, Note: "ANALYTICS_PRIVATE_NOTE_SENTINEL"}))
	readGoal(t, f.privateRequest(t, a, "POST", "/goals/"+g.ID+"/achieve", goalVersion(g), goals.Transition{CriteriaConfirmed: true, Reason: "Результат проверен"}))
	f.clock.Advance(24 * time.Hour)
	f.loginAs(t, a, "analytics-outcomes@example.test")
	response := f.privateRequest(t, a, "POST", "/analytics/rebuild", "", analytics.Range{From: date, To: "2026-10-13"})
	if strings.Contains(string(response.body), "ANALYTICS_PRIVATE_NOTE_SENTINEL") {
		t.Fatal("free-text metric note leaked into analytics")
	}
	v := readAnalytics(t, response)
	d := v.Days[0]
	if d.Planned != 7 || d.PlannedCompleted != 3 || d.PlannedFailed != 1 || d.PlannedSkipped != 1 || d.PlannedPending != 2 || d.Completed != 3 || d.OnTime != 1 || d.Late != 1 || d.NoDeadline != 1 || d.Failed != 1 || d.Skipped != 1 || d.Postponed != 1 || d.PostponementEvents != 2 {
		t.Fatal("wrong task denominators", d)
	}
	if !d.HabitsComplete || d.HabitObligations != 2 || d.HabitSuccesses != 1 || d.HabitMissed != 1 || d.GoalsAchieved != 1 || len(d.Habits) != 2 || d.Plan == nil || !d.Plan.Confirmed || len(d.Plan.Snapshot.Items) != 7 || d.TaskWeightTotal != 14 || d.TaskWeightCompleted != 6 {
		t.Fatal("wrong habit/goal/snapshot basis", d)
	}
	if v.Weeks[0].OutcomeTotals != d.OutcomeTotals {
		t.Fatal("weekly totals disagree", v.Weeks, d.OutcomeTotals)
	}
	found := false
	for _, s := range d.Sources {
		if s.Kind == "goal_metric" {
			found = s.Value == 20 && s.Unit == "страниц" && s.Date == date && s.URL == "/app/goals/"+g.ID
		}
	}
	if !found {
		t.Fatal("goal units/source lost", d.Sources)
	}
}
