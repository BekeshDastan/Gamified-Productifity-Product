//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

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
func TestGoalQuestEventsTreeAndRollback(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "goal-events@example.test")
	g := readGoal(t, f.privateRequest(t, client, "POST", "/goals", "", goals.Create{Input: goalInput("Дерево"), State: "active"}))
	root := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput("Контейнер")))
	childInput := questInput("Обязательный лист")
	childInput.ParentID = &root.ID
	child := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", childInput))
	optionalInput := childInput
	optionalInput.Title = "Необязательный"
	optionalInput.Required = false
	_ = readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", optionalInput))
	g = readGoal(t, f.privateRequest(t, client, "PUT", "/goals/"+g.ID+"/links/"+root.ID, goalVersion(g), goals.LinkInput{Required: true, Weight: 1}))
	child = readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+child.ID+"/complete", version(child), map[string]any{}, "goal-leaf-complete-key"))
	g = readGoal(t, f.privateRequest(t, client, "GET", "/goals/"+g.ID, "", nil))
	if g.Progress != 100 || g.State != "active" {
		t.Fatal("leaf or automatic achievement", g)
	}
	leafVersion := child.Version
	_ = readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+child.ID+"/complete", fmt.Sprintf(`"%d"`, leafVersion-1), map[string]any{}, "goal-leaf-complete-key"))
	repeated := readGoal(t, f.privateRequest(t, client, "GET", "/goals/"+g.ID, "", nil))
	if repeated.Version != g.Version {
		t.Fatal("replay changed goal")
	}
	child = readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+child.ID+"/reopen", version(child), map[string]any{}))
	g = readGoal(t, f.privateRequest(t, client, "GET", "/goals/"+g.ID, "", nil))
	if g.Progress != 0 {
		t.Fatal("reopen not reflected")
	}
	// Both source event and projection must roll back if goal history cannot persist.
	_, e := f.pool.Exec(t.Context(), `CREATE FUNCTION reject_goal_history() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test projection failure'; END $$; CREATE TRIGGER reject_goal_history BEFORE INSERT ON goal_versions FOR EACH ROW EXECUTE FUNCTION reject_goal_history()`)
	if e != nil {
		t.Fatal(e)
	}
	r := f.privateRequest(t, client, "POST", "/quests/"+child.ID+"/complete", version(child), map[string]any{})
	if r.status != 500 {
		t.Fatal("failure not propagated", r.status)
	}
	fresh := readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+child.ID, "", nil))
	if fresh.Status != "todo" || fresh.Version != child.Version {
		t.Fatal("quest committed without projection")
	}
	_, e = f.pool.Exec(t.Context(), `DROP TRIGGER reject_goal_history ON goal_versions; DROP FUNCTION reject_goal_history()`)
	if e != nil {
		t.Fatal(e)
	}
	extra := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput("Другая ветка")))
	otherGoal := readGoal(t, f.privateRequest(t, client, "POST", "/goals", "", goals.Create{Input: goalInput("Другая"), State: "active"}))
	otherGoal = readGoal(t, f.privateRequest(t, client, "PUT", "/goals/"+otherGoal.ID+"/links/"+extra.ID, goalVersion(otherGoal), goals.LinkInput{Required: true, Weight: 1}))
	extra.ParentID = &root.ID
	if r = f.privateRequest(t, client, "PATCH", "/quests/"+extra.ID, version(extra), extra.Input); r.status != 422 {
		t.Fatal("overlap via parent move", r.status, string(r.body))
	}
	r = f.privateRequest(t, client, "GET", "/quests?goal_id="+g.ID, "", nil)
	var page struct{ Data struct{ Total int } }
	if r.status != 200 || json.Unmarshal(r.body, &page) != nil || page.Data.Total != 3 {
		t.Fatal("goal filter", r.status, string(r.body))
	}
}
func TestGoalDecisionsCorrectionReviewAndArchive(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "goal-decisions@example.test")
	input := goalInput("Книги")
	input.ProgressType = "metric"
	input.Unit = "книги"
	input.Target = 2
	g := readGoal(t, f.privateRequest(t, client, "POST", "/goals", "", goals.Create{Input: input, State: "draft"}))
	transition := goals.Transition{Reason: "Начинаю"}
	apply := func(action string) goals.Goal {
		return readGoal(t, f.privateRequest(t, client, "POST", "/goals/"+g.ID+"/"+action, goalVersion(g), transition))
	}
	g = apply("resume")
	g = apply("pause")
	g = apply("resume")
	transition.CriteriaConfirmed = true
	if r := f.privateRequest(t, client, "POST", "/goals/"+g.ID+"/achieve", goalVersion(g), transition); r.status != 422 {
		t.Fatal("premature achievement")
	}
	entry := goals.MetricInput{Value: 2, Unit: "книги", OccurredOn: "2026-10-12"}
	g = readGoal(t, f.privateRequest(t, client, "POST", "/goals/"+g.ID+"/entries", goalVersion(g), entry))
	if g.State != "active" {
		t.Fatal("auto achieved")
	}
	transition.CriteriaConfirmed = false
	if r := f.privateRequest(t, client, "POST", "/goals/"+g.ID+"/achieve", goalVersion(g), transition); r.status != 422 {
		t.Fatal("criterion unconfirmed")
	}
	transition.CriteriaConfirmed = true
	old := goalVersion(g)
	g = readGoal(t, f.privateRequest(t, client, "POST", "/goals/"+g.ID+"/achieve", old, transition, "goal-achievement-replay"))
	repeated := readGoal(t, f.privateRequest(t, client, "POST", "/goals/"+g.ID+"/achieve", old, transition, "goal-achievement-replay"))
	if g.State != "achieved" || repeated.Version != g.Version {
		t.Fatal("achievement replay")
	}
	var entries struct{ Data []goals.Entry }
	r := f.privateRequest(t, client, "GET", "/goals/"+g.ID+"/entries", "", nil)
	if json.Unmarshal(r.body, &entries) != nil || len(entries.Data) != 1 {
		t.Fatal("entries")
	}
	entry.Value = 1
	entry.Reason = "Одна книга ещё не прочитана"
	entry.EntryVersion = entries.Data[0].Version
	g = readGoal(t, f.privateRequest(t, client, "PATCH", fmt.Sprintf("/goals/%s/entries/%d", g.ID, entries.Data[0].ID), goalVersion(g), entry))
	if g.State != "active" || g.Progress != 50 {
		t.Fatal("achievement not reverted")
	}
	var history struct{ Data []goals.Version }
	r = f.privateRequest(t, client, "GET", "/goals/"+g.ID+"/history", "", nil)
	if json.Unmarshal(r.body, &history) != nil || history.Data[0].Kind != "goal.achievement_reverted" {
		t.Fatal("reversal history", string(r.body))
	}
	transition.Completed = "Прочитана одна книга"
	transition.Obstacles = "Вторая требует больше времени"
	g = apply("close-unachieved")
	if g.State != "not_achieved" {
		t.Fatal("unachieved")
	}
	g = apply("archive")
	if g.ArchivedAt == nil {
		t.Fatal("archive")
	}
	if r = f.privateRequest(t, client, "POST", "/goals/"+g.ID+"/resume", goalVersion(g), transition); r.status != 409 {
		t.Fatal("archived mutation")
	}
	g = apply("restore")
	next := g.Input
	next.Criteria = "Прочитать и написать конспекты"
	date := "2026-11-30"
	next.TargetDate = &date
	transition.Resolution = "continue"
	transition.NextInput = &next
	transition.Reason = "Уточнил объём"
	g = apply("review")
	if g.State != "active" || g.Criteria != next.Criteria || g.Progress != 50 || g.ArchivedAt != nil {
		t.Fatal("review")
	}
	r = f.privateRequest(t, client, "GET", "/goals/"+g.ID+"/history", "", nil)
	if json.Unmarshal(r.body, &history) != nil {
		t.Fatal("history")
	}
	var oldest goals.Goal
	if json.Unmarshal(history.Data[len(history.Data)-1].Snapshot, &oldest) != nil || oldest.Criteria != input.Criteria {
		t.Fatal("criteria history overwritten")
	}
	transition.NextInput = nil
	transition.Resolution = ""
	transition.Reason = "Изменились планы"
	g = apply("cancel")
	g = apply("archive")
	if g.State != "cancelled" || g.ArchivedAt == nil {
		t.Fatal("cancel archive")
	}
}
func TestGoalConcurrentEditsAndSourceControl(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "goal-control@example.test")
	g := readGoal(t, f.privateRequest(t, client, "POST", "/goals", "", goals.Create{Input: goalInput("Контроль"), State: "active"}))
	statuses := make(chan int, 2)
	for _, title := range []string{"Первое окно", "Второе окно"} {
		go func() {
			input := g.Input
			input.Title = title
			statuses <- f.privateRequest(t, client, "PATCH", "/goals/"+g.ID, goalVersion(g), goals.Edit{Input: input, Reason: "Уточнение"}).status
		}()
	}
	a, b := <-statuses, <-statuses
	if !(a == 200 && b == 409 || a == 409 && b == 200) {
		t.Fatal("concurrent edit", a, b)
	}
	g = readGoal(t, f.privateRequest(t, client, "GET", "/goals/"+g.ID, "", nil))
	g = readGoal(t, f.privateRequest(t, client, "POST", "/goals/"+g.ID+"/milestones", goalVersion(g), goals.MilestoneInput{Title: "Группа", Required: true, Weight: 100}))
	var plan struct{ Data goals.Plan }
	planResponse := f.privateRequest(t, client, "GET", "/goals/"+g.ID+"/plan", "", nil)
	if json.Unmarshal(planResponse.body, &plan) != nil || len(plan.Data.Milestones) != 1 {
		t.Fatal("plan group")
	}
	for i, w := range []float64{3, 1} {
		q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput(fmt.Sprintf("Лист %d", i))))
		link := goals.LinkInput{Required: true, Weight: w}
		if i == 0 {
			link.MilestoneID = &plan.Data.Milestones[0].ID
		}
		g = readGoal(t, f.privateRequest(t, client, "PUT", "/goals/"+g.ID+"/links/"+q.ID, goalVersion(g), link))
		if i == 0 {
			_ = readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]any{}))
			g = readGoal(t, f.privateRequest(t, client, "GET", "/goals/"+g.ID, "", nil))
		}
	}
	var expected float64
	if e := f.pool.QueryRow(t.Context(), `SELECT 100*sum(l.weight) FILTER(WHERE q.status='completed')/sum(l.weight) FROM goal_quests l JOIN quests q ON q.owner_id=l.owner_id AND q.id=l.quest_id WHERE l.goal_id=$1 AND l.required`, g.ID).Scan(&expected); e != nil || expected != 75 || g.Progress != expected {
		t.Fatal("source control", expected, g.Progress, e)
	}
	other, _ := f.signedIn(t, "goal-history-owner@example.test")
	if r := f.privateRequest(t, other, "GET", "/goals/"+g.ID+"/history", "", nil); r.status != 404 {
		t.Fatal("foreign history")
	}
	// Rebuild removes live associations, keeps source quests and immutable past plan.
	next := g.Input
	date := "2026-12-01"
	next.TargetDate = &date
	g = readGoal(t, f.privateRequest(t, client, "POST", "/goals/"+g.ID+"/review", goalVersion(g), goals.Transition{Reason: "Новая стратегия", Resolution: "rebuild", NextInput: &next}))
	if g.Progress != 0 || g.State != "active" {
		t.Fatal("rebuild progress")
	}
	var count int
	if e := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM quests`).Scan(&count); e != nil || count != 2 {
		t.Fatal("rebuild deleted quests", count, e)
	}
	var history struct{ Data []goals.Version }
	r := f.privateRequest(t, client, "GET", "/goals/"+g.ID+"/history", "", nil)
	if json.Unmarshal(r.body, &history) != nil {
		t.Fatal("history")
	}
	var snapshot struct {
		Plan goals.Plan `json:"plan"`
	}
	if json.Unmarshal(history.Data[1].Snapshot, &snapshot) != nil || len(snapshot.Plan.Links) != 2 {
		t.Fatal("rebuild erased prior plan")
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
func TestGoalDeadlineUsesOwnerCalendarWithoutClosing(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "goal-timezone@example.test")
	input := goalInput("Календарный срок")
	day := "2026-10-12"
	input.TargetDate = &day
	g := readGoal(t, f.privateRequest(t, client, "POST", "/goals", "", goals.Create{Input: input, State: "active"}))
	initialVersion := g.Version
	for _, tc := range []struct {
		zone    string
		overdue bool
	}{{"UTC", false}, {"Pacific/Kiritimati", true}, {"America/New_York", false}} {
		if _, e := f.pool.Exec(t.Context(), `UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}',to_jsonb($2::text)) WHERE owner_id=$1`, account.ID, tc.zone); e != nil {
			t.Fatal(e)
		}
		g = readGoal(t, f.privateRequest(t, client, "GET", "/goals/"+g.ID, "", nil))
		if g.Overdue != tc.overdue || g.TargetDate == nil || *g.TargetDate != day || g.State != "active" || g.Version != initialVersion {
			t.Fatal("deadline", tc.zone, g)
		}
	}
	// Account clock can cross midnight without a write or an automatic failure.
	f.clock.Advance(24 * time.Hour)
	refreshed := f.request(t, client, "POST", "/refresh", f.csrf(t, client), struct{}{})
	if refreshed.status != 200 {
		t.Fatal("refresh after day", refreshed.status)
	}
	g = readGoal(t, f.privateRequest(t, client, "GET", "/goals/"+g.ID, "", nil))
	if !g.Overdue || g.State != "active" || g.Version != initialVersion {
		t.Fatal("automatic closing", g)
	}
}
func TestGoalMetricPrecisionAtLargeBaseline(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "goal-precision@example.test")
	input := goalInput("Точный прирост")
	input.ProgressType = "metric"
	input.Unit = "единицы"
	input.Start = 999999999.99
	input.Target = 1000000000
	g := readGoal(t, f.privateRequest(t, client, "POST", "/goals", "", goals.Create{Input: input, State: "active"}))
	g = readGoal(t, f.privateRequest(t, client, "POST", "/goals/"+g.ID+"/entries", goalVersion(g), goals.MetricInput{Value: 0.001, Unit: input.Unit, OccurredOn: "2026-10-12"}))
	if g.Progress != 10 {
		t.Fatal("numeric precision", g.Progress)
	}
}
