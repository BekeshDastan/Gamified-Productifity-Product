//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"testing"
	"time"
)

func readPlan(t *testing.T, r authResponse) planning.Plan {
	t.Helper()
	var out struct {
		Data planning.Plan `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatalf("plan %d %s", r.status, r.body)
	}
	return out.Data
}
func TestPlanningDashboardCrossViewAndEmptyAccount(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "dashboard-plan@example.test")
	b, _ := f.signedIn(t, "empty-dashboard@example.test")
	read := func(r authResponse) planning.Dashboard {
		var out struct {
			Data planning.Dashboard `json:"data"`
		}
		if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
			t.Fatalf("dashboard %d %s", r.status, r.body)
		}
		return out.Data
	}
	empty := read(f.privateRequest(t, b, "GET", "/dashboard", "", nil))
	if empty.Plan.ID != "" || len(empty.Tasks) != 0 || len(empty.Deadlines) != 0 || len(empty.Events) != 0 {
		t.Fatal("invented data")
	}
	date := "2026-10-12"
	input := questInput("Живая задача")
	input.PlannedDate = &date
	deadline := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	input.Deadline = &deadline
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	path := "/plans/day/" + date
	p := readPlan(t, f.privateRequest(t, a, "PUT", path, `"0"`, planning.Input{AvailableMinutes: 60, QuestIDs: []string{q.ID}, TopIDs: []string{q.ID}}))
	readPlan(t, f.privateRequest(t, a, "POST", path+"/confirm", planVersion(p), struct{}{}))
	before := read(f.privateRequest(t, a, "GET", "/dashboard?date="+date, "", nil))
	if len(before.Tasks) != 1 || len(before.Deadlines) != 1 || before.Plan.Progress.Committed != 1 {
		t.Fatal("missing current data", before)
	}
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", fmt.Sprintf(`"%d"`, q.Version), map[string]string{"reason": "Готово"}))
	after := read(f.privateRequest(t, a, "GET", "/dashboard?date="+date, "", nil))
	if after.Tasks[0].Status != "completed" || len(after.Deadlines) != 0 || after.Plan.Progress.Completed != 1 {
		t.Fatal("stale cross-view result", after)
	}
	if len(read(f.privateRequest(t, b, "GET", "/dashboard", "", nil)).Tasks) != 0 {
		t.Fatal("foreign owner leak")
	}
	if r := f.privateRequest(t, a, "GET", "/dashboard?date=invalid", "", nil); r.status != 422 {
		t.Fatal("invalid date", r.status)
	}
}
func readRoutinePreview(t *testing.T, r authResponse) planning.RoutinePreview {
	t.Helper()
	var out struct {
		Data planning.RoutinePreview `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil || out.Data.Hash == "" {
		t.Fatalf("preview %d %s", r.status, r.body)
	}
	return out.Data
}
func TestPlanningCustomRoutinesAndAtomicFailures(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "custom-routines@example.test")
	b, _ := f.signedIn(t, "foreign-routines@example.test")
	read := func(r authResponse) planning.Routine {
		var out struct {
			Data planning.Routine `json:"data"`
		}
		if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
			t.Fatalf("routine %d %s", r.status, r.body)
		}
		return out.Data
	}
	in := planning.RoutineInput{Title: "Личный вечер", Period: "day", Days: []planning.RoutineDay{{Day: 1, AvailableMinutes: 60, Actions: []planning.RoutineAction{{Key: "read", Title: "Читать", Priority: "normal", Minutes: 30}}}}}
	custom := read(f.privateRequest(t, a, "POST", "/templates/routines", "", in))
	in.Title = "Новый вечер"
	custom = read(f.privateRequest(t, a, "PATCH", "/templates/routines/"+custom.ID, `"1"`, in))
	if r := f.privateRequest(t, a, "PATCH", "/templates/routines/"+custom.ID, `"1"`, in); r.status != 409 {
		t.Fatal("stale routine", r.status)
	}
	if r := f.privateRequest(t, b, "POST", "/templates/routines/"+custom.ID+"/preview", "", map[string]string{"start_date": "2026-10-13"}); r.status != 404 {
		t.Fatal("foreign routine", r.status)
	}
	read(f.privateRequest(t, a, "PATCH", "/templates/routines/study-day", `"1"`, in))
	if r := f.privateRequest(t, a, "GET", "/templates/routines", "", nil); r.status != 200 {
		t.Fatal("catalog", r.status)
	}
	preview := readRoutinePreview(t, f.privateRequest(t, a, "POST", "/templates/routines/"+custom.ID+"/preview", "", map[string]string{"start_date": "2026-10-13"}))
	readRoutinePreview(t, f.privateRequest(t, a, "POST", "/templates/routines/"+custom.ID+"/apply", "", planning.RoutineRequest{StartDate: preview.StartDate, PreviewHash: preview.Hash, Reason: "Мой вечер"}))
	var key string
	if e := f.pool.QueryRow(t.Context(), `SELECT id::text FROM quests WHERE payload->>'title'='Читать'`).Scan(&key); e != nil {
		t.Fatal(e)
	}
	p := readPlan(t, f.privateRequest(t, a, "GET", "/plans/day/2026-10-13", "", nil))
	q := readQuest(t, f.privateRequest(t, a, "GET", "/quests/"+key, "", nil))
	readPlan(t, f.privateRequest(t, a, "POST", "/plans/day/2026-10-13/move", planVersion(p), planning.Move{QuestID: key, QuestVersion: q.Version, Reason: "Планирую вечер"}))
	p = readPlan(t, f.privateRequest(t, a, "GET", "/plans/day/2026-10-13", "", nil))
	q = readQuest(t, f.privateRequest(t, a, "GET", "/quests/"+key, "", nil))
	if _, e := f.pool.Exec(t.Context(), `CREATE FUNCTION reject_planning_block() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected rollback'; END $$; CREATE TRIGGER reject_planning_block BEFORE INSERT ON calendar_events FOR EACH ROW EXECUTE FUNCTION reject_planning_block()`); e != nil {
		t.Fatal(e)
	}
	r := f.privateRequest(t, a, "POST", "/plans/day/2026-10-13/blocks", planVersion(p), planning.Block{Move: planning.Move{QuestID: key, QuestVersion: q.Version, Reason: "Проверка атомарности"}, Start: "19:00", End: "20:00"})
	if r.status != 500 {
		t.Fatal("expected rollback", r.status)
	}
	fresh := readPlan(t, f.privateRequest(t, a, "GET", "/plans/day/2026-10-13", "", nil))
	if fresh.Version != p.Version {
		t.Fatal("plan committed after block failure")
	}
	if r = f.privateRequest(t, a, "POST", "/plans/day/2026-10-13/addendum", planVersion(p), planning.Amendment{Input: p.Input, Reason: "Ещё не подтверждён"}); r.status != 409 {
		t.Fatal("draft amendment", r.status)
	}
	if r = f.privateRequest(t, a, "POST", "/templates/routines/ideal-week/preview", "", map[string]string{"start_date": "2026-10-13"}); r.status != 422 {
		t.Fatal("week starts Tuesday", r.status)
	}
}
func TestPlanningRoutinePreviewDedupeAndConflicts(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "routine-plan@example.test")
	series := calendar.SeriesInput{Event: calendar.Input{Title: "Повторяемая встреча", Start: "2026-10-12T09:30:00Z", End: "2026-10-12T10:30:00Z", Timezone: "UTC"}, Kind: "event", Rule: calendar.Rule{Frequency: "daily", Interval: 1, Count: 2, Weekdays: []int{}}}
	readSeries(t, f.privateRequest(t, client, "POST", "/calendar/series", "", series))
	path := "/templates/routines/ideal-week"
	preview := readRoutinePreview(t, f.privateRequest(t, client, "POST", path+"/preview", "", map[string]string{"start_date": "2026-10-12"}))
	if len(preview.Days) != 7 || len(preview.Days[0].Availability.Conflicts) != 1 {
		t.Fatal("projected series missing", preview.Days)
	}
	var n int
	if e := f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM quests)+(SELECT count(*) FROM calendar_events)+(SELECT count(*) FROM day_plans)`).Scan(&n); e != nil || n != 0 {
		t.Fatal("preview wrote data", n, e)
	}
	apply := planning.RoutineRequest{StartDate: preview.StartDate, PreviewHash: preview.Hash, Reason: "Выбираю неделю"}
	if r := f.privateRequest(t, client, "POST", path+"/apply", "", apply); r.status != 422 {
		t.Fatal("hidden conflicts", r.status)
	}
	apply.AcceptConflicts = true
	preview = readRoutinePreview(t, f.privateRequest(t, client, "POST", path+"/apply", "", apply, "routine-apply-once-key"))
	if !preview.Days[0].AlreadyApplied || !preview.Days[6].Input.Rest {
		t.Fatal("not applied")
	}
	if e := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM quests`).Scan(&n); e != nil || n != 6 {
		t.Fatal("week tasks", n, e)
	}
	apply.PreviewHash = preview.Hash
	readRoutinePreview(t, f.privateRequest(t, client, "POST", path+"/apply", "", apply))
	if e := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM quests`).Scan(&n); e != nil || n != 6 {
		t.Fatal("duplicate week", n, e)
	}
	stale := readRoutinePreview(t, f.privateRequest(t, client, "POST", "/templates/routines/study-day/preview", "", map[string]string{"start_date": "2026-10-20"}))
	readEvent(t, f.privateRequest(t, client, "POST", "/calendar/events", "", calendar.Input{Title: "Новая встреча", Start: "2026-10-20T09:00:00Z", End: "2026-10-20T10:00:00Z", Timezone: "UTC"}))
	if r := f.privateRequest(t, client, "POST", "/templates/routines/study-day/apply", "", planning.RoutineRequest{StartDate: stale.StartDate, PreviewHash: stale.Hash, Reason: "Устаревший план", AcceptConflicts: true}); r.status != 409 {
		t.Fatal("stale preview applied", r.status)
	}
}

func TestPlanningWeekShowsDraftsAndRestWithoutInventedPlans(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "week-plan@example.test")
	path := "/plans/day/2026-10-12"
	p := readPlan(t, f.privateRequest(t, client, "PUT", path, `"0"`, planning.Input{Rest: true}))
	readPlan(t, f.privateRequest(t, client, "POST", path+"/confirm", planVersion(p), struct{}{}))
	r := f.privateRequest(t, client, "GET", "/plans/week?from=2026-10-12", "", nil)
	var out struct {
		Data []planning.Plan `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil || len(out.Data) != 7 {
		t.Fatal("week", r.status, string(r.body))
	}
	if !out.Data[0].Rest || out.Data[0].Progress.Score != nil || out.Data[1].ID != "" || out.Data[1].ConfirmedAt != nil {
		t.Fatal("invented commitment")
	}
	var n int
	if e := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM day_plans`).Scan(&n); e != nil || n != 1 {
		t.Fatal("week mutated plans", n, e)
	}
}

func TestPlanningAvailabilityUsesRealCalendar(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "capacity-plan@example.test")
	qIn := questInput("Нужны два часа")
	qIn.EstimatedMinutes = 120
	q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", qIn))
	in := planInput()
	in.QuestIDs = []string{q.ID}
	path := "/plans/day/2026-10-12"
	readPlan(t, f.privateRequest(t, client, "PUT", path, `"0"`, in))
	for _, v := range []struct{ start, end string }{{"09:00", "10:00"}, {"09:30", "10:30"}, {"10:30", "11:00"}} {
		readEvent(t, f.privateRequest(t, client, "POST", "/calendar/events", "", calendar.Input{Title: "Занято", Start: "2026-10-12T" + v.start + ":00Z", End: "2026-10-12T" + v.end + ":00Z", Timezone: "UTC"}))
	}
	p := readPlan(t, f.privateRequest(t, client, "GET", path, "", nil))
	a := p.Availability
	if a == nil || a.BusyMinutes != 120 || a.FreeMinutes != 60 || a.DeficitMinutes != 60 || len(a.Conflicts) != 1 || len(a.UnfitIDs) != 1 {
		t.Fatal("calendar not reflected", a)
	}
}

func TestPlanningPrioritiesMultipleBlocksAndDeadline(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "blocks-plan@example.test")
	qIn := questInput("Два блока работы")
	deadline := f.clock.Now().Add(72 * time.Hour)
	qIn.Deadline = &deadline
	q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", qIn))
	in := planInput()
	in.QuestIDs = []string{q.ID}
	in.TopIDs = []string{q.ID}
	path := "/plans/day/2026-10-13"
	p := readPlan(t, f.privateRequest(t, client, "PUT", path, `"0"`, in))
	first := planning.Block{Move: planning.Move{QuestID: q.ID, QuestVersion: q.Version, Reason: "Утренний блок"}, Start: "09:00", End: "10:00"}
	readEvent(t, f.privateRequest(t, client, "POST", path+"/blocks", planVersion(p), first, "planning-first-block"))
	readEvent(t, f.privateRequest(t, client, "POST", path+"/blocks", planVersion(p), first, "planning-first-block"))
	p = readPlan(t, f.privateRequest(t, client, "GET", path, "", nil))
	q = readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+q.ID, "", nil))
	if q.PlannedDate == nil || *q.PlannedDate != "2026-10-13" || !q.Deadline.Equal(deadline) {
		t.Fatal("date or deadline changed", q)
	}
	second := first
	second.QuestVersion = q.Version
	second.Start = "14:00"
	second.End = "15:00"
	readEvent(t, f.privateRequest(t, client, "POST", path+"/blocks", planVersion(p), second))
	p = readPlan(t, f.privateRequest(t, client, "GET", path, "", nil))
	p = readPlan(t, f.privateRequest(t, client, "POST", path+"/confirm", planVersion(p), struct{}{}))
	if len(p.Snapshot.Blocks) != 2 {
		t.Fatal("blocks lost or duplicated", len(p.Snapshot.Blocks))
	}
	var ids []string
	for i := 0; i < 4; i++ {
		q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput(fmt.Sprintf("Приоритет %d", i))))
		ids = append(ids, q.ID)
	}
	in.QuestIDs = ids
	in.TopIDs = ids
	if r := f.privateRequest(t, client, "PUT", "/plans/day/2026-10-14", `"0"`, in); r.status != 422 {
		t.Fatal("four priorities accepted", r.status)
	}
}
func planInput() planning.Input {
	return planning.Input{AvailableMinutes: 120, Windows: []planning.Window{{Start: "09:00", End: "12:00"}}, QuestIDs: []string{}, TopIDs: []string{}}
}
func planVersion(p planning.Plan) string { return fmt.Sprintf(`"%d"`, p.Version) }
func TestPlanningDraftOwnershipAndVersion(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "plan@example.test")
	other, _ := f.signedIn(t, "other-plan@example.test")
	path := "/plans/day/2026-10-12"
	empty := readPlan(t, f.privateRequest(t, client, "GET", path, "", nil))
	if empty.Version != 0 || empty.ConfirmedAt != nil {
		t.Fatal("invented commitment")
	}
	p := readPlan(t, f.privateRequest(t, client, "PUT", path, `"0"`, planInput()))
	if p.Version != 1 || p.ID == "" || p.Snapshot != nil {
		t.Fatal("draft", p)
	}
	if r := f.privateRequest(t, client, "PUT", path, `"0"`, planInput()); r.status != 409 {
		t.Fatal("duplicate overwrite", r.status)
	}
	empty = readPlan(t, f.privateRequest(t, other, "GET", path, "", nil))
	if empty.ID != "" {
		t.Fatal("owner leak")
	}
	var n int
	if e := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM day_plans`).Scan(&n); e != nil || n != 1 {
		t.Fatal("duplicate plan", n, e)
	}
	in := planInput()
	in.TopIDs = []string{"00000000-0000-4000-8000-000000000000"}
	if r := f.privateRequest(t, client, "PUT", path, planVersion(p), in); r.status != 422 {
		t.Fatal("invalid top accepted", r.status)
	}
}

func TestPlanningFrozenSnapshotAndAddenda(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "snapshot-plan@example.test")
	a := questInput("Важное действие")
	a.Priority = "high"
	a.EstimatedMinutes = 90
	q1 := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", a))
	a.Title = "Малое действие"
	a.Priority = "low"
	a.EstimatedMinutes = 30
	q2 := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", a))
	in := planInput()
	in.QuestIDs = []string{q1.ID, q2.ID}
	in.TopIDs = []string{q1.ID}
	path := "/plans/day/2026-10-12"
	p := readPlan(t, f.privateRequest(t, client, "PUT", path, `"0"`, in))
	p = readPlan(t, f.privateRequest(t, client, "POST", path+"/confirm", planVersion(p), struct{}{}))
	if p.Snapshot == nil || p.Progress.TotalWeight != 4 || p.Progress.Committed != 2 || p.Progress.Score == nil || *p.Progress.Score != 0 {
		t.Fatal("snapshot missing", p)
	}
	if r := f.privateRequest(t, client, "PUT", path, planVersion(p), planInput()); r.status != 409 {
		t.Fatal("confirmed overwritten", r.status)
	}
	in.QuestIDs = []string{q1.ID}
	p = readPlan(t, f.privateRequest(t, client, "POST", path+"/addendum", planVersion(p), planning.Amendment{Input: in, Reason: "Меньше времени"}))
	if len(p.Snapshot.Items) != 2 || p.Progress.TotalWeight != 4 || len(p.QuestIDs) != 1 || len(p.Changes) != 2 {
		t.Fatal("denominator changed", p)
	}
	readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+q1.ID+"/complete", version(q1), map[string]string{"reason": "Готово"}))
	p = readPlan(t, f.privateRequest(t, client, "GET", path, "", nil))
	if p.Progress.CompletedWeight != 3 || *p.Progress.Score != 75 {
		t.Fatal("weighted snapshot", p.Progress)
	}
	f.clock.Advance(24 * 60 * 60 * 1e9)
	if r := f.request(t, client, "POST", "/refresh", f.csrf(t, client), struct{}{}); r.status != 200 {
		t.Fatal("refresh after advancing clock", r.status)
	}
	readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+q2.ID+"/complete", version(q2), map[string]string{"reason": "На следующий день"}))
	p = readPlan(t, f.privateRequest(t, client, "GET", path, "", nil))
	if *p.Progress.Score != 75 {
		t.Fatal("late completion changed prior day")
	}
	rest := planning.Input{Rest: true}
	r := readPlan(t, f.privateRequest(t, client, "PUT", "/plans/day/2026-10-14", `"0"`, rest))
	r = readPlan(t, f.privateRequest(t, client, "POST", "/plans/day/2026-10-14/confirm", planVersion(r), struct{}{}))
	if r.Progress.Score != nil {
		t.Fatal("rest scored")
	}
}
