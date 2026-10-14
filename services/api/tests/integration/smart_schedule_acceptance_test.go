//go:build integration

package integration

import (
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"testing"
	"time"
)

func TestSmartScheduleAllDayUnknownCapacityAndStaleTaskOrDependency(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "schedule-edge@example.test")
	input := questInput("Работа")
	input.EstimatedMinutes = 45
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	in := planning.ScheduleInput{From: "2026-10-13", To: "2026-10-14", Tasks: []planning.TaskOption{{ID: q.ID}}}
	v := readSchedule(t, f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in))
	if len(v.Placements) != 0 || v.Unscheduled[0].FreeMinutes != 0 {
		t.Fatal("invented work day", v)
	}
	readPlan(t, f.privateRequest(t, a, "PUT", "/plans/day/2026-10-13", `"0"`, planning.Input{AvailableMinutes: 120, Windows: []planning.Window{{Start: "09:00", End: "11:00"}}}))
	fixed := readEvent(t, f.privateRequest(t, a, "POST", "/calendar/events", "", calendar.Input{Title: "Фиксированный день", AllDay: true, Start: "2026-10-13", End: "2026-10-14", Timezone: "UTC"}))
	v = readSchedule(t, f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in))
	if len(v.Placements) != 0 || v.Unscheduled[0].FreeMinutes != 0 {
		t.Fatal("all-day conflict ignored", v)
	}
	in.Breaks = []planning.Interval{{Start: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC)}}
	if r := f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in); r.status != 422 {
		t.Fatal("break overlapping fixed accepted", r.status)
	}
	in.Breaks = nil
	readEvent(t, f.privateRequest(t, a, "PATCH", "/calendar/events/"+fixed.ID, eventVersion(fixed), calendar.Edit{Input: fixed.Input, Cancelled: true, Reason: "Изменён день"}))
	v = readSchedule(t, f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in))
	if len(v.Placements) != 1 {
		t.Fatal(v)
	}
	apply := planning.ScheduleApply{OperationID: testkit.NewUser().ID, PreviewID: v.ID, Confirm: true}
	q.Title = "Исправленная оценка"
	q = readQuest(t, f.privateRequest(t, a, "PATCH", "/quests/"+q.ID, version(q), q.Input))
	if r := f.privateRequest(t, a, "POST", "/plans/schedule/apply", "", apply); r.status != 409 {
		t.Fatal("stale task", r.status)
	}
	v = readSchedule(t, f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in))
	dependency := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Зависимость")))
	q = readQuest(t, f.privateRequest(t, a, "PUT", "/quests/"+q.ID+"/dependencies", version(q), quests.DependenciesInput{IDs: []string{dependency.ID}}))
	apply.PreviewID = v.ID
	if r := f.privateRequest(t, a, "POST", "/plans/schedule/apply", "", apply); r.status != 409 {
		t.Fatal("stale dependency", r.status)
	}
	otherInput := questInput("Пять минут начала")
	otherInput.EstimatedMinutes = 45
	other := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", otherInput))
	in.Tasks = []planning.TaskOption{{ID: other.ID, FiveMinuteStart: true}}
	v = readSchedule(t, f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in))
	if len(v.Placements) != 1 || v.Placements[0].Minutes != 5 || !v.Placements[0].PartialStart {
		t.Fatal(v)
	}
	apply = planning.ScheduleApply{OperationID: testkit.NewUser().ID, PreviewID: v.ID, Confirm: false}
	if r := f.privateRequest(t, a, "POST", "/plans/schedule/apply", "", apply); r.status != 422 {
		t.Fatal("consent ignored", r.status)
	}
}
