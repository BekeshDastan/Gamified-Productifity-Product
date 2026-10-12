//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"testing"
	"time"
)

func habitInput() habits.Input {
	return habits.Input{Title: "Читать", Kind: "count", Unit: "страниц", Minimum: 1, StartsOn: "2026-10-12", Schedule: habits.Schedule{Frequency: "daily", Weekdays: []int{1, 2, 3, 4, 5}, Target: 20}}
}
func readHabit(t *testing.T, r authResponse) habits.Habit {
	t.Helper()
	var out struct {
		Data habits.Habit `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatalf("habit %d %s", r.status, r.body)
	}
	return out.Data
}
func habitVersion(h habits.Habit) string { return fmt.Sprintf(`"%d"`, h.Version) }
func TestHabitTypedModelsAndFutureSchedules(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "habits-model@example.test")
	b, _ := f.signedIn(t, "habits-other@example.test")
	for _, kind := range []string{"binary", "count", "duration"} {
		in := habitInput()
		in.Kind = kind
		if kind == "binary" {
			in.Schedule.Target = 1
		}
		if kind == "duration" {
			in.Unit = "мин"
		}
		h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", in))
		if h.Kind != kind || h.Timezone == "" || len(h.Schedules) != 1 {
			t.Fatal(h)
		}
		if r := f.privateRequest(t, b, "GET", "/habits/"+h.ID, "", nil); r.status != 404 {
			t.Fatal("foreign", r.status)
		}
		in.Minimum = 0
		if r := f.privateRequest(t, a, "POST", "/habits", "", in); r.status != 422 {
			t.Fatal("zero", r.status)
		}
	}
	h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", habitInput()))
	change := habits.ScheduleChange{EffectiveFrom: "2026-10-19", Schedule: habits.Schedule{Frequency: "weekly", Target: 100}, Reason: "Новая норма со следующей недели"}
	h2 := readHabit(t, f.privateRequest(t, a, "POST", "/habits/"+h.ID+"/schedules", habitVersion(h), change))
	if h2.Version != 2 || len(h2.Schedules) != 2 || h2.Schedules[0].Schedule.Target != 20 || h2.Schedules[1].Schedule.Target != 100 {
		t.Fatal("schedule history", h2)
	}
	if r := f.privateRequest(t, a, "POST", "/habits/"+h.ID+"/schedules", habitVersion(h), change); r.status != 409 {
		t.Fatal("stale", r.status)
	}
	change.EffectiveFrom = "2026-10-12"
	if r := f.privateRequest(t, a, "POST", "/habits/"+h.ID+"/schedules", habitVersion(h2), change); r.status != 422 {
		t.Fatal("retroactive", r.status)
	}
	_ = time.UTC
}
