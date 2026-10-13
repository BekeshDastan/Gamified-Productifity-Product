//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/journal"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"testing"
	"time"
)

func readWeekResult(t *testing.T, r authResponse) journal.WeekResult {
	t.Helper()
	var out struct{ Data journal.WeekResult }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestJournalWeeklyApplyPermanentReplayAndNoDuplicateTasks(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "reset-apply@example.test")
	date := "2026-10-05"
	inQ := questInput("Перенести осознанно")
	inQ.PlannedDate = &date
	deadline := f.clock.Now().Add(-time.Hour)
	inQ.Deadline = &deadline
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", inQ))
	archived := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Больше не нужно")))
	path := "/reviews/week/2026-10-12"
	preview := readWeekPreview(t, f.privateRequest(t, a, "POST", path+"/preview", "", struct{}{}))
	target := "2026-10-19"
	input := journal.WeekApplyInput{PreviewID: preview.ID, Changes: []journal.WeekChange{{ID: q.ID, Version: q.Version, Action: "move", PlannedDate: &target, Reason: "Выбрал следующую неделю"}, {ID: archived.ID, Version: archived.Version, Action: "archive", Reason: "Неактуально"}}, Plans: []journal.WeekPlan{{Date: target, Version: 0, Input: planning.Input{AvailableMinutes: 60, QuestIDs: []string{q.ID}, TopIDs: []string{q.ID}}}}, Reason: "Следующая неделя"}
	applied := readWeekResult(t, f.privateRequest(t, a, "POST", path+"/apply-plan", `"0"`, input))
	if applied.Version != 1 || len(applied.ChangedIDs) != 2 || len(applied.PlanIDs) != 1 {
		t.Fatal(applied)
	}
	moved := readQuest(t, f.privateRequest(t, a, "GET", "/quests/"+q.ID, "", nil))
	if moved.PlannedDate == nil || *moved.PlannedDate != target || moved.Status != "postponed" || !moved.Deadline.Equal(deadline) {
		t.Fatal(moved)
	}
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "reset-apply@example.test")
	replay := readWeekResult(t, f.privateRequest(t, a, "POST", path+"/apply-plan", `"0"`, input))
	if replay.Version != applied.Version || !replay.AppliedAt.Equal(applied.AppliedAt) {
		t.Fatal("repeat changed result", replay)
	}
	var n int
	for table, want := range map[string]int{"quests": 2, "day_plans": 1, "weekly_reviews": 1} {
		if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table+" WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != want {
			t.Fatal(table, n, e)
		}
	}
	input.Reason = "Другое решение"
	if r := f.privateRequest(t, a, "POST", path+"/apply-plan", `"1"`, input); r.status != 409 {
		t.Fatal("changed repeat", r.status)
	}
}
func TestJournalWeeklyStaleExpiryAndAtomicPlanFailure(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "reset-stale@example.test")
	b, _ := f.signedIn(t, "reset-foreign@example.test")
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Мой шаг")))
	foreign := readQuest(t, f.privateRequest(t, b, "POST", "/quests", "", questInput("Чужой шаг")))
	path := "/reviews/week/2026-10-12"
	preview := readWeekPreview(t, f.privateRequest(t, a, "POST", path+"/preview", "", struct{}{}))
	target := "2026-10-19"
	input := journal.WeekApplyInput{PreviewID: preview.ID, Changes: []journal.WeekChange{{ID: q.ID, Version: q.Version, Action: "move", PlannedDate: &target, Reason: "Выбрал дату"}}, Plans: []journal.WeekPlan{{Date: target, Input: planning.Input{AvailableMinutes: 30, QuestIDs: []string{foreign.ID}}}}, Reason: "Обзор"}
	if r := f.privateRequest(t, a, "POST", path+"/apply-plan", `"0"`, input); r.status != 404 {
		t.Fatal("foreign plan", r.status, string(r.body))
	}
	unchanged := readQuest(t, f.privateRequest(t, a, "GET", "/quests/"+q.ID, "", nil))
	if unchanged.Version != q.Version || unchanged.PlannedDate != nil {
		t.Fatal("partial write", unchanged)
	}
	if r := f.privateRequest(t, b, "POST", path+"/apply-plan", `"0"`, input); r.status != 404 {
		t.Fatal("foreign preview", r.status)
	}
	input.Plans = []journal.WeekPlan{}
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/start", fmt.Sprintf(`"%d"`, q.Version), map[string]string{"reason": "Готов к работе"}))
	if r := f.privateRequest(t, a, "POST", path+"/apply-plan", `"0"`, input); r.status != 409 {
		t.Fatal("stale", r.status, string(r.body))
	}
	fresh := readWeekPreview(t, f.privateRequest(t, a, "POST", path+"/preview", "", struct{}{}))
	input.PreviewID = fresh.ID
	input.Changes = nil
	f.clock.Advance(10 * time.Minute)
	if r := f.privateRequest(t, a, "POST", path+"/apply-plan", `"0"`, input); r.status != 409 {
		t.Fatal("expired", r.status)
	}
}
