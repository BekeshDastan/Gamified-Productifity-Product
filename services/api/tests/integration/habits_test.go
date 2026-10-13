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
		Data habits.LogPage `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatalf("logs %d %s", r.status, r.body)
	}
	return out.Data.Items
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
func TestHabitFuturePausesAndEmergencyCannotHidePast(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "habit-pause@example.test")
	h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", habitInput()))
	path := "/habits/" + h.ID
	end := "2026-10-14"
	h = readHabit(t, f.privateRequest(t, a, "POST", path+"/pauses", habitVersion(h), habits.PauseInput{StartsOn: "2026-10-13", EndsOn: &end, Reason: "Плановый отдых"}))
	if len(h.Pauses) != 1 {
		t.Fatal(h)
	}
	h = readHabit(t, f.privateRequest(t, a, "POST", path+"/pauses/"+h.Pauses[0].ID+"/resume", habitVersion(h), habits.ResumeInput{ResumesOn: "2026-10-14", Reason: "Возвращаюсь раньше"}))
	if *h.Pauses[0].EndsOn != "2026-10-13" {
		t.Fatal(h.Pauses)
	}
	f.clock.Advance(48 * time.Hour)
	if r := f.request(t, a, "POST", "/refresh", f.csrf(t, a), struct{}{}); r.status != 200 {
		t.Fatal(r.status)
	}
	h = readHabit(t, f.privateRequest(t, a, "GET", path, "", nil))
	if len(h.Periods) != 3 || h.Periods[0].State != "missed" || h.Periods[1].State != "rest" || h.Periods[1].RestReason == "" || h.Periods[2].State != "open" {
		t.Fatal(h.Periods)
	}
	if r := f.privateRequest(t, a, "POST", path+"/emergency", habitVersion(h), habits.EmergencyInput{PeriodID: h.Periods[0].ID, Reason: "Попытка скрыть прошлое"}); r.status != 422 {
		t.Fatal("retro emergency", r.status)
	}
	if r := f.privateRequest(t, a, "POST", path+"/pauses", habitVersion(h), habits.PauseInput{StartsOn: "2026-10-12", Reason: "Поздняя пауза"}); r.status != 422 {
		t.Fatal("retro pause", r.status)
	}
	h = readHabit(t, f.privateRequest(t, a, "POST", path+"/emergency", habitVersion(h), habits.EmergencyInput{PeriodID: h.Periods[2].ID, Reason: "Непредвиденное обстоятельство"}))
	if h.Periods[2].State != "rest" || h.Periods[2].EmergencyReason == "" || h.Stats.Successful != 0 || h.Stats.Missed != 1 {
		t.Fatal("emergency counted as success or hid past", h)
	}
}
func TestHabitArchivePreviewAndPersistentTimezone(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, account := f.signedIn(t, "habit-history@example.test")
	h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", habitInput()))
	path := "/habits/" + h.ID
	in := habits.ScheduleChange{EffectiveFrom: "2026-10-19", Schedule: habits.Schedule{Frequency: "weekly", Target: 100}, Reason: "Новая недельная норма"}
	r := f.privateRequest(t, a, "POST", path+"/schedules/preview", habitVersion(h), in)
	var preview struct {
		Data habits.SchedulePreview `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &preview) != nil || preview.Data.Proposed.Target != 100 || preview.Data.FirstEnd.Format(time.DateOnly) != "2026-10-26" {
		t.Fatal("preview", r.status, string(r.body))
	}
	fresh := readHabit(t, f.privateRequest(t, a, "GET", path, "", nil))
	if fresh.Version != h.Version || len(fresh.Schedules) != 1 {
		t.Fatal("preview wrote state")
	}
	h = readHabit(t, f.privateRequest(t, a, "POST", path+"/archive", habitVersion(h), habits.ReasonInput{Reason: "Убираю на время"}))
	if h.ArchivedAt == nil || len(h.Archives) != 1 {
		t.Fatal(h)
	}
	f.clock.Advance(48 * time.Hour)
	if r := f.request(t, a, "POST", "/refresh", f.csrf(t, a), struct{}{}); r.status != 200 {
		t.Fatal(r.status)
	}
	if _, e := f.pool.Exec(t.Context(), `UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}',to_jsonb('Pacific/Kiritimati'::text)) WHERE owner_id=$1`, account.ID); e != nil {
		t.Fatal(e)
	}
	h = readHabit(t, f.privateRequest(t, a, "GET", path, "", nil))
	if len(h.Periods) != 1 || h.Timezone != "UTC" || h.Periods[0].Key != "daily:2026-10-12" {
		t.Fatal("archive generated or timezone rewrote", h)
	}
	h = readHabit(t, f.privateRequest(t, a, "POST", path+"/restore", habitVersion(h), habits.ReasonInput{Reason: "Возвращаюсь"}))
	if h.ArchivedAt != nil || h.Archives[0].RestoredAt == nil || len(h.Periods) != 1 {
		t.Fatal("retro obligations after restore", h)
	}
	f.clock.Advance(24 * time.Hour)
	if r := f.request(t, a, "POST", "/refresh", f.csrf(t, a), struct{}{}); r.status != 200 {
		t.Fatal(r.status)
	}
	h = readHabit(t, f.privateRequest(t, a, "GET", path, "", nil))
	if len(h.Periods) != 2 || h.Periods[1].Key != "daily:2026-10-15" {
		t.Fatal(h.Periods)
	}
}
func TestHabitMaterializeContinuationAndHistoryPagination(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, account := f.signedIn(t, "habit-long@example.test")
	in := habitInput()
	in.StartsOn = "2024-10-12"
	in.Schedule.Weekdays = []int{1, 2, 3, 4, 5, 6, 7}
	h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", in))
	if !h.HasMore {
		t.Fatal("silent materialization cutoff")
	}
	h = readHabit(t, f.privateRequest(t, a, "GET", "/habits/"+h.ID, "", nil))
	if h.HasMore || len(h.Periods) != 120 || h.Stats.Missed < 700 {
		t.Fatal("continuation or stats", h.Stats, len(h.Periods), h.HasMore)
	}
	readPage := func(path string) habits.PeriodPage {
		r := f.privateRequest(t, a, "GET", path, "", nil)
		var out struct {
			Data habits.PeriodPage `json:"data"`
		}
		if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
			t.Fatal(r.status, string(r.body))
		}
		return out.Data
	}
	path := "/habits/" + h.ID
	first := readPage(path + "/periods")
	if len(first.Items) != 60 || first.NextCursor == nil {
		t.Fatal(first)
	}
	second := readPage(path + "/periods?before=" + *first.NextCursor)
	if len(second.Items) != 60 || !second.Items[0].Start.Before(first.Items[59].Start) {
		t.Fatal("period pagination", second)
	}
	p := h.Periods[len(h.Periods)-1]
	for i := 0; i < 105; i++ {
		if _, e := f.pool.Exec(t.Context(), `INSERT INTO habit_logs(owner_id,habit_id,period_id,amount,original_amount,occurred_at,accepted_at,backfilled) VALUES($1,$2,$3,0.1,0.1,$4,$4,false)`, account.ID, h.ID, p.ID, f.clock.Now()); e != nil {
			t.Fatal(e)
		}
	}
	r := f.privateRequest(t, a, "GET", path+"/logs", "", nil)
	var logs struct {
		Data habits.LogPage `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &logs) != nil || len(logs.Data.Items) != 100 || logs.Data.NextCursor == nil {
		t.Fatal("log page", r.status, string(r.body))
	}
	cursor := *logs.Data.NextCursor
	r = f.privateRequest(t, a, "GET", path+"/logs?before="+cursor, "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &logs) != nil || len(logs.Data.Items) != 5 || logs.Data.NextCursor != nil {
		t.Fatal("log tail", r.status, string(r.body))
	}
}
