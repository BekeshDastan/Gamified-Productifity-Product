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
func TestHabitPeriodsDedupeAndTimezone(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "habits-periods@example.test")
	for _, freq := range []string{"daily", "weekly", "monthly"} {
		in := habitInput()
		in.Schedule.Frequency = freq
		if freq != "daily" {
			in.Schedule.Weekdays = nil
		}
		h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", in))
		if len(h.Periods) != 1 || h.Periods[0].State != "open" {
			t.Fatal(freq, h)
		}
		p := h.Periods[0]
		again := readHabit(t, f.privateRequest(t, a, "GET", "/habits/"+h.ID, "", nil))
		if len(again.Periods) != 1 || again.Periods[0].ID != p.ID {
			t.Fatal("duplicate", freq)
		}
		if freq == "weekly" && p.End.Format(time.DateOnly) != "2026-10-19" {
			t.Fatal("early weekly end", p.End)
		}
		if freq == "monthly" && p.End.Format(time.DateOnly) != "2026-11-01" {
			t.Fatal("early monthly end", p.End)
		}
	}
	f.clock.Advance(24 * time.Hour)
	if r := f.request(t, a, "POST", "/refresh", f.csrf(t, a), struct{}{}); r.status != 200 {
		t.Fatal(r.status)
	}
	var out struct {
		Data []habits.Habit `json:"data"`
	}
	r := f.privateRequest(t, a, "GET", "/habits", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	for _, h := range out.Data {
		if h.Schedule.Frequency == "daily" {
			if len(h.Periods) != 2 || h.Periods[0].State != "missed" || h.Periods[1].State != "open" {
				t.Fatal(h.Periods)
			}
		} else if len(h.Periods) != 1 || h.Periods[0].State != "open" {
			t.Fatal("week/month closed early", h.Periods)
		}
	}
}
func readHabitLogs(t *testing.T, r authResponse) []habits.Log {
	t.Helper()
	var out struct {
		Data []habits.Log `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatalf("logs %d %s", r.status, r.body)
	}
	return out.Data
}
func TestHabitQuantitativeLogsAndCorrections(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "habit-logs@example.test")
	b, _ := f.signedIn(t, "habit-logs-other@example.test")
	h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", habitInput()))
	path := "/habits/" + h.ID
	in := habits.LogInput{PeriodID: h.Periods[0].ID, Amount: 10, OccurredAt: f.clock.Now().Add(-time.Hour)}
	key := "habit-log-replay-0001"
	v := habitVersion(h)
	h = readHabit(t, f.privateRequest(t, a, "POST", path+"/logs", v, in, key))
	if h.Periods[0].Amount != 10 || h.Periods[0].Progress != 50 || h.Periods[0].State != "open" {
		t.Fatal(h.Periods)
	}
	replay := readHabit(t, f.privateRequest(t, a, "POST", path+"/logs", v, in, key))
	if replay.Version != h.Version || replay.Periods[0].Amount != 10 {
		t.Fatal("double log", replay)
	}
	in.Amount = -10
	if r := f.privateRequest(t, a, "POST", path+"/logs", habitVersion(h), in); r.status != 422 {
		t.Fatal("negative normal log", r.status)
	}
	in.Amount = 10
	h = readHabit(t, f.privateRequest(t, a, "POST", path+"/logs", habitVersion(h), in))
	if h.Periods[0].Amount != 20 || h.Periods[0].State != "success" {
		t.Fatal(h.Periods)
	}
	logs := readHabitLogs(t, f.privateRequest(t, a, "GET", path+"/logs", "", nil))
	if len(logs) != 2 || logs[0].Backfilled {
		t.Fatal(logs)
	}
	corr := habits.CorrectionInput{Amount: 0, Reason: "Ошибочная отметка"}
	h = readHabit(t, f.privateRequest(t, a, "POST", path+"/logs/"+logs[0].ID+"/correct", habitVersion(h), corr))
	if h.Periods[0].Amount != 10 || h.Periods[0].Progress != 50 {
		t.Fatal("correction projection", h.Periods)
	}
	logs = readHabitLogs(t, f.privateRequest(t, a, "GET", path+"/logs", "", nil))
	if logs[0].OriginalAmount != 10 || logs[0].Amount != 0 || len(logs[0].Corrections) != 1 || logs[0].Corrections[0].Before != 10 {
		t.Fatal("correction history", logs)
	}
	if r := f.privateRequest(t, b, "POST", path+"/logs", habitVersion(h), in); r.status != 404 {
		t.Fatal("foreign log", r.status)
	}
	f.clock.Advance(24 * time.Hour)
	if r := f.request(t, a, "POST", "/refresh", f.csrf(t, a), struct{}{}); r.status != 200 {
		t.Fatal(r.status)
	}
	h = readHabit(t, f.privateRequest(t, a, "GET", path, "", nil))
	h = readHabit(t, f.privateRequest(t, a, "POST", path+"/logs", habitVersion(h), in))
	logs = readHabitLogs(t, f.privateRequest(t, a, "GET", path+"/logs", "", nil))
	if !logs[0].Backfilled || h.Periods[0].Amount != 20 {
		t.Fatal("backfilled fact", logs)
	}
}
func TestHabitStreakWeekendAndBestCorrection(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "habit-streak@example.test")
	in := habitInput()
	in.StartsOn = "2026-10-09"
	in.Schedule.Weekdays = []int{1, 3, 5}
	h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", in))
	if len(h.Periods) != 2 {
		t.Fatal("weekend obligation", h.Periods)
	}
	path := "/habits/" + h.ID
	first := h.Periods[0]
	last := h.Periods[1]
	h = readHabit(t, f.privateRequest(t, a, "POST", path+"/logs", habitVersion(h), habits.LogInput{PeriodID: first.ID, Amount: 20, OccurredAt: first.Start.Add(time.Hour)}))
	if h.Stats.Current != 1 || h.Stats.Best != 1 || h.Stats.Missed != 0 {
		t.Fatal("open or weekend broke series", h.Stats)
	}
	h = readHabit(t, f.privateRequest(t, a, "POST", path+"/logs", habitVersion(h), habits.LogInput{PeriodID: last.ID, Amount: 20, OccurredAt: last.Start.Add(time.Hour)}))
	if h.Stats.Current != 2 || h.Stats.Best != 2 {
		t.Fatal(h.Stats)
	}
	logs := readHabitLogs(t, f.privateRequest(t, a, "GET", path+"/logs", "", nil))
	var old habits.Log
	for _, l := range logs {
		if l.PeriodID == first.ID {
			old = l
		}
	}
	h = readHabit(t, f.privateRequest(t, a, "POST", path+"/logs/"+old.ID+"/correct", habitVersion(h), habits.CorrectionInput{Amount: 0, Reason: "Исправляю прошлый факт"}))
	if h.Stats.Current != 1 || h.Stats.Best != 1 || h.Stats.Missed != 1 || !h.Stats.Recovering {
		t.Fatal("best retained false run", h.Stats)
	}
}
