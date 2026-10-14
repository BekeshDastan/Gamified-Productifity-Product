//go:build integration

package integration

import (
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"testing"
	"time"
)

func TestSmartScheduleExplicitBreakReplayMismatchAndPlanContext(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "schedule-context@example.test")
	input := questInput("Разместить")
	input.EstimatedMinutes = 30
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	plan := planning.Input{AvailableMinutes: 180, Windows: []planning.Window{{Start: "13:00", End: "16:00"}}}
	readPlan(t, f.privateRequest(t, a, "PUT", "/plans/day/2026-10-13", `"0"`, plan))
	in := planning.ScheduleInput{From: "2026-10-13", To: "2026-10-14", Tasks: []planning.TaskOption{{ID: q.ID}}, Breaks: []planning.Interval{{Start: time.Date(2026, 10, 13, 13, 15, 0, 0, time.UTC), End: time.Date(2026, 10, 13, 13, 45, 0, 0, time.UTC)}}, BetweenMinutes: 5}
	v := readSchedule(t, f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in))
	if len(v.Placements) != 1 || v.Placements[0].Start.Format("15:04") != "13:45" {
		t.Fatal(v)
	}
	apply := planning.ScheduleApply{OperationID: testkit.NewUser().ID, PreviewID: v.ID, Confirm: true}
	result := readScheduleResult(t, f.privateRequest(t, a, "POST", "/plans/schedule/apply", "", apply))
	if len(result.Blocks) != 3 {
		t.Fatal("explicit break not saved", result)
	}
	apply.PreviewID = testkit.NewUser().ID
	if r := f.privateRequest(t, a, "POST", "/plans/schedule/apply", "", apply); r.status != 409 {
		t.Fatal("request identity changed", r.status)
	}
	in.Tasks = []planning.TaskOption{{ID: q.ID}}
	in.Breaks = nil
	v = readSchedule(t, f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in))
	apply = planning.ScheduleApply{OperationID: testkit.NewUser().ID, PreviewID: v.ID, Confirm: true}
	if r := f.privateRequest(t, a, "POST", "/plans/schedule/apply", "", apply); r.status != 409 {
		t.Fatal("empty proposal applied", r.status)
	}
	other := questInput("Новый шаг")
	other.EstimatedMinutes = 30
	q2 := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", other))
	in.Tasks = []planning.TaskOption{{ID: q2.ID}}
	in.Breaks = nil
	v = readSchedule(t, f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in))
	current := readPlan(t, f.privateRequest(t, a, "GET", "/plans/day/2026-10-13", "", nil))
	plan = current.Input
	plan.AvailableMinutes = 160
	readPlan(t, f.privateRequest(t, a, "POST", "/plans/day/2026-10-13/addendum", planVersion(current), planning.Amendment{Input: plan, Reason: "Другая ёмкость"}))
	apply = planning.ScheduleApply{OperationID: testkit.NewUser().ID, PreviewID: v.ID, Confirm: true}
	if r := f.privateRequest(t, a, "POST", "/plans/schedule/apply", "", apply); r.status != 409 {
		t.Fatal("plan version ignored", r.status)
	}
	in.Tasks = []planning.TaskOption{{ID: testkit.NewUser().ID}}
	if r := f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in); r.status != 404 {
		t.Fatal("foreign task", r.status)
	}
	in.Tasks = []planning.TaskOption{{ID: q2.ID}}
	if _, e := f.pool.Exec(f.ctx, `UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}',to_jsonb('Europe/Berlin'::text)),version=version+1 WHERE owner_id=$1`, who.ID); e != nil {
		t.Fatal(e)
	}
	if r := f.privateRequest(t, a, "POST", "/plans/schedule/preview", "", in); r.status != 409 {
		t.Fatal("old frozen plan zone", r.status)
	}
}
