//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"sync"
	"testing"
	"time"
)

func readSchedule(t *testing.T, r authResponse) planning.SchedulePreview {
	t.Helper()
	var out struct{ Data planning.SchedulePreview }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func readScheduleResult(t *testing.T, r authResponse) planning.ScheduleResult {
	t.Helper()
	var out struct{ Data planning.ScheduleResult }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestSmartScheduleAtomicRacePermanentReplayAndFrozenCommitments(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "schedule@example.test")
	b, _ := f.signedIn(t, "schedule-other@example.test")
	date := "2026-10-12"
	p := readPlan(t, f.privateRequest(t, a, "PUT", "/plans/day/"+date, `"0"`, planning.Input{AvailableMinutes: 180, Windows: []planning.Window{{Start: "13:00", End: "16:00"}}}))
	p = readPlan(t, f.privateRequest(t, a, "POST", "/plans/day/"+date+"/confirm", planVersion(p), struct{}{}))
	fixed := readEvent(t, f.privateRequest(t, a, "POST", "/calendar/events", "", calendar.Input{Title: "Фиксированная встреча", Start: date + "T14:00:00Z", End: date + "T14:30:00Z", Timezone: "UTC"}))
	input := questInput("Важный шаг")
	input.EstimatedMinutes = 30
	deadline := time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)
	input.Deadline = &deadline
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	input = questInput("Делимый шаг")
	input.EstimatedMinutes = 45
	q2 := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	in := planning.ScheduleInput{From: date, To: "2026-10-13", Tasks: []planning.TaskOption{{ID: q.ID}, {ID: q2.ID, AllowSplit: true}}, BetweenMinutes: 5}
	preview := readSchedule(t, f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in))
	if len(preview.Placements) != 3 || preview.Placements[0].TaskID != q.ID || len(preview.InputVersions) < 4 {
		t.Fatal(preview)
	}
	var count int
	f.pool.QueryRow(f.ctx, `SELECT count(*) FROM calendar_events WHERE owner_id=$1`, who.ID).Scan(&count)
	if count != 1 {
		t.Fatal("preview mutated calendar", count)
	}
	apply := planning.ScheduleApply{OperationID: testkit.NewUser().ID, PreviewID: preview.ID, Confirm: true}
	if r := f.privateRequest(t, b, "POST", "/plans/schedule/apply", "", apply); r.status != 404 {
		t.Fatal("foreign preview", r.status)
	}
	replies := make([]authResponse, 4)
	var wg sync.WaitGroup
	for i := range replies {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			replies[i] = f.privateRequest(t, a, "POST", "/plans/schedule/apply", "", apply)
		}(i)
	}
	wg.Wait()
	saved := readScheduleResult(t, replies[0])
	for _, r := range replies {
		if readScheduleResult(t, r).OperationID != saved.OperationID {
			t.Fatal("duplicate operation")
		}
	}
	f.pool.QueryRow(f.ctx, `SELECT count(*) FROM calendar_events WHERE owner_id=$1`, who.ID).Scan(&count)
	if count != 1+len(saved.Blocks) {
		t.Fatal(count, len(saved.Blocks))
	}
	current := readPlan(t, f.privateRequest(t, a, "GET", "/plans/day/"+date, "", nil))
	if current.Snapshot == nil || len(current.Snapshot.Items) != 0 || len(current.QuestIDs) != 2 || len(current.Changes) == 0 {
		t.Fatal("original commitments rewritten", current)
	}
	actual := readQuest(t, f.privateRequest(t, a, "GET", "/quests/"+q.ID, "", nil))
	if actual.Deadline == nil || !actual.Deadline.Equal(deadline) || actual.PlannedDate == nil || *actual.PlannedDate != date {
		t.Fatal(actual)
	}
	var got calendar.Event
	for _, ev := range readCalendar(t, f.privateRequest(t, a, "GET", "/calendar?from=2026-10-12&to=2026-10-13", "", nil)).Items {
		if ev.ID == fixed.ID {
			got = ev
		}
	}
	if got.Version != fixed.Version || got.Start != fixed.Start {
		t.Fatal("fixed event changed")
	}
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "schedule@example.test")
	replayed := readScheduleResult(t, f.privateRequest(t, a, "POST", "/plans/schedule/apply", "", apply))
	if len(replayed.Blocks) != len(saved.Blocks) {
		t.Fatal("permanent replay expired")
	}
	apply.OperationID = testkit.NewUser().ID
	if r := f.privateRequest(t, a, "POST", "/plans/schedule/apply", "", apply); r.status != 409 {
		t.Fatal("preview used twice", r.status)
	}
}
func TestSmartScheduleStaleBusyDependenciesExpiryAndUnknown(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "schedule-stale@example.test")
	date := "2026-10-12"
	readPlan(t, f.privateRequest(t, a, "PUT", "/plans/day/"+date, `"0"`, planning.Input{AvailableMinutes: 120, Windows: []planning.Window{{Start: "13:00", End: "15:00"}}}))
	input := questInput("Шаг")
	input.EstimatedMinutes = 30
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	in := planning.ScheduleInput{From: date, To: "2026-10-13", Tasks: []planning.TaskOption{{ID: q.ID}}}
	preview := readSchedule(t, f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in))
	apply := planning.ScheduleApply{OperationID: testkit.NewUser().ID, PreviewID: preview.ID, Confirm: true}
	readEvent(t, f.privateRequest(t, a, "POST", "/calendar/events", "", calendar.Input{Title: "Новая занятость", Start: date + "T13:00:00Z", End: date + "T14:00:00Z", Timezone: "UTC"}))
	if r := f.privateRequest(t, a, "POST", "/plans/schedule/apply", "", apply); r.status != 409 {
		t.Fatal("stale busy accepted", r.status, string(r.body))
	}
	unknown := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Без оценки")))
	q = readQuest(t, f.privateRequest(t, a, "PUT", "/quests/"+q.ID+"/dependencies", version(q), quests.DependenciesInput{IDs: []string{unknown.ID}}))
	in.Tasks = []planning.TaskOption{{ID: q.ID}, {ID: unknown.ID}}
	preview = readSchedule(t, f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in))
	if len(preview.Placements) != 0 || len(preview.Unscheduled) != 2 {
		t.Fatal(preview)
	}
	in.Tasks = []planning.TaskOption{{ID: unknown.ID, FiveMinuteStart: true}}
	preview = readSchedule(t, f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in))
	if len(preview.Placements) != 0 || preview.Unscheduled[0].Reason != "unknown_estimate" {
		t.Fatal(preview)
	}
	apply.PreviewID = preview.ID
	f.clock.Advance(11 * time.Minute)
	if r := f.privateRequest(t, a, "POST", "/plans/schedule/apply", "", apply); r.status != 409 {
		t.Fatal("expired", r.status)
	}
	in.To = "2026-10-28"
	if r := f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in); r.status != 422 {
		t.Fatal("unbounded", r.status)
	}
}
