//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
)

func goalInput(title string) goals.Input {
	return goals.Input{Title: title, Criteria: "Обязательные результаты", ProgressType: "quests", Target: 1}
}

func TestGoalPlanWeightsTransferAndOwnership(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "goal-plan@example.test")
	other, _ := f.signedIn(t, "foreign-plan@example.test")
	create := func(title string) goals.Goal {
		return readGoal(t, f.privateRequest(t, client, "POST", "/goals", "", goals.Create{Input: goalInput(title), State: "active"}))
	}
	a, b := create("Первая"), create("Вторая")
	q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput("Результат")))
	q = readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]any{}))
	link := goals.LinkInput{Required: true, Weight: 3}
	a = readGoal(t, f.privateRequest(t, client, "PUT", "/goals/"+a.ID+"/links/"+q.ID, goalVersion(a), link))
	if a.Progress != 100 {
		t.Fatal("linked completion not counted")
	}
	if r := f.privateRequest(t, client, "PUT", "/goals/"+b.ID+"/links/"+q.ID, goalVersion(b), link); r.status != 409 {
		t.Fatal("silent transfer")
	}
	link.PreviousGoalID = &a.ID
	link.PreviousGoalVersion = a.Version
	link.Reason = "Перенёс в другой план"
	b = readGoal(t, f.privateRequest(t, client, "PUT", "/goals/"+b.ID+"/links/"+q.ID, goalVersion(b), link))
	a = readGoal(t, f.privateRequest(t, client, "GET", "/goals/"+a.ID, "", nil))
	if a.Progress != 0 || b.Progress != 100 {
		t.Fatal("transfer summaries differ")
	}
	foreign := readQuest(t, f.privateRequest(t, other, "POST", "/quests", "", questInput("Чужая")))
	if r := f.privateRequest(t, client, "PUT", "/goals/"+b.ID+"/links/"+foreign.ID, goalVersion(b), link); r.status != 404 {
		t.Fatal("foreign link")
	}
	q2 := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput("Ещё результат")))
	link.PreviousGoalID = nil
	link.PreviousGoalVersion = 0
	link.Weight = 1
	b = readGoal(t, f.privateRequest(t, client, "PUT", "/goals/"+b.ID+"/links/"+q2.ID, goalVersion(b), link))
	if b.Progress != 75 {
		t.Fatal("weighted leaves", b.Progress)
	}
	in := goalInput("Этапы")
	in.ProgressType = "milestones"
	m := readGoal(t, f.privateRequest(t, client, "POST", "/goals", "", goals.Create{Input: in, State: "active"}))
	m = readGoal(t, f.privateRequest(t, client, "POST", "/goals/"+m.ID+"/milestones", goalVersion(m), goals.MilestoneInput{Title: "Готово", Required: true, Weight: 3, Completed: true}))
	m = readGoal(t, f.privateRequest(t, client, "POST", "/goals/"+m.ID+"/milestones", goalVersion(m), goals.MilestoneInput{Title: "Дальше", Required: true, Weight: 1}))
	if m.Progress != 75 {
		t.Fatal("milestone weights", m.Progress)
	}
}
func readGoal(t *testing.T, r authResponse) goals.Goal {
	t.Helper()
	var v struct {
		Data goals.Goal `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &v) != nil || v.Data.ID == "" {
		t.Fatalf("goal %d %s", r.status, r.body)
	}
	return v.Data
}
func goalVersion(g goals.Goal) string { return fmt.Sprintf(`"%d"`, g.Version) }
func TestGoalTemplatePreviewApplyAndReplay(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "goal-template@example.test")
	var catalog struct {
		Data []goals.GoalTemplate `json:"data"`
	}
	r := f.privateRequest(t, client, "GET", "/templates/goals", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &catalog) != nil || len(catalog.Data) < 6 {
		t.Fatal("catalog", r.status, string(r.body))
	}
	date := "2026-12-25"
	source := catalog.Data[1]
	source.Input.StartDate = &date
	in := goals.TemplateDraft{TemplateID: source.ID, Input: source.Input, DurationDays: 30, Milestones: source.Milestones, Tasks: source.Tasks}
	in.Tasks[0].Title = "Мой первый шаг"
	r = f.privateRequest(t, client, "POST", "/templates/goals/preview", "", in)
	var preview struct {
		Data goals.GoalPreview `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &preview) != nil || *preview.Data.Input.TargetDate != "2027-01-23" || preview.Data.Tasks[1].PlannedDate != "2027-01-04" {
		t.Fatal("preview dates", r.status, string(r.body))
	}
	var count int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM goals`).Scan(&count); err != nil || count != 0 {
		t.Fatal("preview persisted", count, err)
	}
	if r = f.privateRequest(t, client, "POST", "/templates/goals/apply", "", in); r.status != 422 {
		t.Fatal("confirmation missing")
	}
	in.Confirmed = true
	g := readGoal(t, f.privateRequest(t, client, "POST", "/templates/goals/apply", "", in, "goal-template-replay-key"))
	repeated := readGoal(t, f.privateRequest(t, client, "POST", "/templates/goals/apply", "", in, "goal-template-replay-key"))
	if g.ID != repeated.ID || g.State != "draft" || g.Progress != 0 {
		t.Fatal("template replay")
	}
	r = f.privateRequest(t, client, "GET", "/goals/"+g.ID+"/plan", "", nil)
	var plan struct {
		Data goals.Plan `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &plan) != nil || len(plan.Data.Links) != 3 || len(plan.Data.Milestones) != 3 || plan.Data.Links[0].Title == "" {
		t.Fatal("template plan", r.status, string(r.body))
	}
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM quests`).Scan(&count); err != nil || count != 3 {
		t.Fatal("tasks duplicated", count, err)
	}
	in.DurationDays = 2
	if r = f.privateRequest(t, client, "POST", "/templates/goals/preview", "", in); r.status != 422 {
		t.Fatal("tasks outside plan")
	}
}
func TestGoalMetricUnitJournalCorrectionAndReplay(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "goal-metric@example.test")
	in := goalInput("Прочитать книги")
	in.ProgressType = "metric"
	in.Start = 2
	in.Target = 12
	in.Unit = "книги"
	g := readGoal(t, f.privateRequest(t, client, "POST", "/goals", "", goals.Create{Input: in, State: "active"}))
	if g.Current != 2 || g.Progress != 0 {
		t.Fatal("numeric start")
	}
	entry := goals.MetricInput{Value: 3, Unit: "минуты", OccurredOn: "2026-10-12", Note: "Три книги"}
	if r := f.privateRequest(t, client, "POST", "/goals/"+g.ID+"/entries", goalVersion(g), entry); r.status != 422 {
		t.Fatal("mixed units")
	}
	entry.Unit = "книги"
	oldVersion := goalVersion(g)
	g = readGoal(t, f.privateRequest(t, client, "POST", "/goals/"+g.ID+"/entries", oldVersion, entry, "goal-metric-replay-key"))
	if g.Current != 5 || g.Progress != 30 {
		t.Fatal("metric percentage", g.Current, g.Progress)
	}
	repeat := readGoal(t, f.privateRequest(t, client, "POST", "/goals/"+g.ID+"/entries", oldVersion, entry, "goal-metric-replay-key"))
	if repeat.Version != g.Version {
		t.Fatal("metric repeated")
	}
	entries := f.privateRequest(t, client, "GET", "/goals/"+g.ID+"/entries", "", nil)
	var result struct {
		Data []goals.Revision `json:"data"`
	}
	if entries.status != 200 || json.Unmarshal(entries.body, &result) != nil || len(result.Data) != 1 {
		t.Fatal("entry journal", string(entries.body))
	}
	v := result.Data[0]
	entry.Value = 1
	entry.EntryVersion = v.Version
	entry.Reason = "Исправил неверное количество"
	g = readGoal(t, f.privateRequest(t, client, "PATCH", fmt.Sprintf("/goals/%s/entries/%d", g.ID, v.ID), goalVersion(g), entry))
	if g.Current != 3 || g.Progress != 10 {
		t.Fatal("correction calculation")
	}
	history := f.privateRequest(t, client, "GET", fmt.Sprintf("/goals/%s/entries/%d/history", g.ID, v.ID), "", nil)
	if history.status != 200 || json.Unmarshal(history.body, &result) != nil || len(result.Data) != 2 || result.Data[1].Value != 3 || result.Data[0].Value != 1 {
		t.Fatal("correction history")
	}
	in.Unit = "минуты"
	if r := f.privateRequest(t, client, "PATCH", "/goals/"+g.ID, goalVersion(g), goals.Edit{Input: in, Reason: "Сменить единицу"}); r.status != 422 {
		t.Fatal("existing entries retyped")
	}
}

func TestGoalCRUDOverdueOwnerAndPromotion(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "goal-owner@example.test")
	other, _ := f.signedIn(t, "goal-other@example.test")
	in := goalInput("Закончить проект")
	day := "2026-10-11"
	in.TargetDate = &day
	g := readGoal(t, f.privateRequest(t, client, "POST", "/goals", "", goals.Create{Input: in, State: "active"}))
	if !g.Overdue || g.State != "active" || g.Progress != 0 {
		t.Fatal("deadline changed state")
	}
	if r := f.privateRequest(t, other, "GET", "/goals/"+g.ID, "", nil); r.status != 404 {
		t.Fatal("foreign goal")
	}
	in.Title = "Новый план"
	if r := f.privateRequest(t, client, "PATCH", "/goals/"+g.ID, goalVersion(g), goals.Edit{Input: in}); r.status != 422 {
		t.Fatal("silent criteria change")
	}
	g = readGoal(t, f.privateRequest(t, client, "PATCH", "/goals/"+g.ID, goalVersion(g), goals.Edit{Input: in, Reason: "Уточнил план"}))
	if r := f.privateRequest(t, client, "PATCH", "/goals/"+g.ID, `"1"`, goals.Edit{Input: in, Reason: "Старый редактор"}); r.status != 409 {
		t.Fatal("lost update")
	}
	if r := f.privateRequest(t, client, "POST", "/goals", "", map[string]any{"owner_id": account.ID}); r.status != 422 {
		t.Fatal("owner accepted")
	}
	in.ProgressType = "metric"
	in.Unit = "книги"
	in.Start = 12
	in.Target = 10
	if r := f.privateRequest(t, client, "POST", "/goals", "", goals.Create{Input: in, State: "active"}); r.status != 422 {
		t.Fatal("decreasing metric")
	}
	if _, e := f.pool.Exec(f.ctx, `INSERT INTO onboarding_starters(owner_id,kind,title,created_at,updated_at) VALUES($1,'goal','Первая цель',$2,$2)`, account.ID, f.clock.Now()); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		r := f.privateRequest(t, client, "GET", "/goals", "", nil)
		var v struct {
			Data goals.Page `json:"data"`
		}
		if r.status != 200 || json.Unmarshal(r.body, &v) != nil || v.Data.Total != 2 {
			t.Fatalf("promotion %d %s", r.status, r.body)
		}
	}
	var count int
	if e := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM goals WHERE payload->>'title'='Первая цель'`).Scan(&count); e != nil || count != 1 {
		t.Fatal("duplicate goal draft")
	}
}
