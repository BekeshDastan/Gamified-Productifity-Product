//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"testing"
)

func calendarInput(title string) calendar.Input {
	return calendar.Input{Title: title, AllDay: true, Start: "2026-10-12", End: "2026-10-13", Timezone: "Asia/Almaty"}
}
func readCalendar(t *testing.T, r authResponse) calendar.Page {
	t.Helper()
	var v struct {
		Data calendar.Page `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &v) != nil {
		t.Fatalf("calendar %d %s", r.status, r.body)
	}
	return v.Data
}
func readEvent(t *testing.T, r authResponse) calendar.Event {
	t.Helper()
	var v struct {
		Data calendar.Event `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &v) != nil || v.Data.ID == "" {
		t.Fatalf("event %d %s", r.status, r.body)
	}
	return v.Data
}
func eventVersion(v calendar.Event) string { return fmt.Sprintf(`"%d"`, v.Version) }
func TestCalendarRecurrenceWindowLimitAndWorkerReplay(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "calendar-series@example.test")
	in := calendar.SeriesInput{Event: calendarInput("Ежедневно"), Kind: "event", Rule: calendar.Rule{Frequency: "daily", Interval: 1, Count: 365, Weekdays: []int{}}}
	in.Event.Start = "2026-01-01"
	in.Event.End = "2026-01-02"
	for i := 0; i < 2; i++ {
		r := f.privateRequest(t, client, "POST", "/calendar/series", "", in)
		if r.status != 200 {
			t.Fatalf("series %d %s", r.status, r.body)
		}
	}
	path := "/calendar?from=2026-01-01&to=2027-01-01"
	first := readCalendar(t, f.privateRequest(t, client, "GET", path, "", nil))
	if !first.Materializing || len(first.Items) != 400 {
		t.Fatal("400 budget", len(first.Items), first.Materializing)
	}
	second := readCalendar(t, f.privateRequest(t, client, "GET", path, "", nil))
	if second.Materializing || len(second.Items) != 730 {
		t.Fatal("continuation", len(second.Items))
	}
	third := readCalendar(t, f.privateRequest(t, client, "GET", path, "", nil))
	if len(third.Items) != 730 {
		t.Fatal("duplicates")
	}
	in.Kind = "task"
	in.Event = calendarInput("Отдельная задача")
	in.Rule.Count = 3
	r := f.privateRequest(t, client, "POST", "/calendar/series", "", in)
	if r.status != 200 {
		t.Fatal("task series", r.status)
	}
	for i := 0; i < 2; i++ {
		svc := calendar.NewService(f.pool, f.clock, quests.NewService(f.pool, f.clock))
		if e := svc.FillHorizon(t.Context()); e != nil {
			t.Fatal(e)
		}
	}
	var tasks, occurrences int
	if e := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM quests WHERE payload->>'title'='Отдельная задача'`).Scan(&tasks); e != nil {
		t.Fatal(e)
	}
	if e := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM calendar_events WHERE quest_id IS NOT NULL`).Scan(&occurrences); e != nil {
		t.Fatal(e)
	}
	if tasks != 3 || occurrences != 3 {
		t.Fatal("worker/task copies", tasks, occurrences)
	}
}
func TestCalendarMoveResizeVersionAndTaskSync(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "calendar-move@example.test")
	q := questInput("План работы")
	original := "2026-10-12"
	q.PlannedDate = &original
	task := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", q))
	in := calendarInput("Работа")
	in.QuestID = &task.ID
	event := readEvent(t, f.privateRequest(t, client, "POST", "/calendar/events", "", in))
	edit := calendar.Edit{Input: in, Reason: "Переношу работу", QuestVersion: task.Version}
	edit.Start = "2026-10-14"
	edit.End = "2026-10-16"
	key := "calendar-move-replay-key"
	moved := readEvent(t, f.privateRequest(t, client, "PATCH", "/calendar/events/"+event.ID, eventVersion(event), edit, key))
	replay := readEvent(t, f.privateRequest(t, client, "PATCH", "/calendar/events/"+event.ID, eventVersion(event), edit, key))
	if moved.Version != 2 || replay.Version != 2 {
		t.Fatal("move replay")
	}
	task = readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+task.ID, "", nil))
	if *task.PlannedDate != "2026-10-14" || task.Status != "postponed" {
		t.Fatal("list not synchronized")
	}
	if r := f.privateRequest(t, client, "PATCH", "/calendar/events/"+event.ID, eventVersion(event), edit); r.status != 409 {
		t.Fatal("stale accepted", r.status)
	}
	edit.QuestVersion = task.Version
	edit.Start = "2026-10-15"
	edit.End = "2026-10-17"
	task = readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+task.ID+"/complete", version(task), map[string]any{}))
	if r := f.privateRequest(t, client, "PATCH", "/calendar/events/"+moved.ID, eventVersion(moved), edit); r.status != 409 {
		t.Fatal("completed history moved")
	}
	persisted := readCalendar(t, f.privateRequest(t, client, "GET", "/calendar?from=2026-10-14&to=2026-10-15", "", nil))
	found := false
	for _, v := range persisted.Items {
		if v.ID == event.ID {
			found = v.Start == "2026-10-14" && v.Status == "completed"
		}
	}
	if !found {
		t.Fatal("failed edit not rolled back")
	}
}
func TestCalendarRangeDatesOwnerAndReplay(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "calendar@example.test")
	other, _ := f.signedIn(t, "calendar-other@example.test")
	in := calendarInput("День без сдвига")
	key := "calendar-create-key-0001"
	a := readEvent(t, f.privateRequest(t, client, "POST", "/calendar/events", "", in, key))
	b := readEvent(t, f.privateRequest(t, client, "POST", "/calendar/events", "", in, key))
	if a.ID != b.ID {
		t.Fatal("replay duplicated event")
	}
	q := questInput("Общая задача")
	q.PlannedDate = &in.Start
	task := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", q))
	in.QuestID = &task.ID
	in.AllDay = false
	in.Start = "2026-10-12T23:00:00+05:00"
	in.End = "2026-10-13T00:00:00+05:00"
	readEvent(t, f.privateRequest(t, client, "POST", "/calendar/events", "", in))
	page := readCalendar(t, f.privateRequest(t, client, "GET", "/calendar?from=2026-10-12&to=2026-10-13", "", nil))
	if len(page.Items) != 3 {
		t.Fatal("date range", len(page.Items))
	}
	var whole calendar.Event
	for _, item := range page.Items {
		if item.ID == a.ID {
			whole = item
		}
	}
	if whole.Start != "2026-10-12" || whole.End != "2026-10-13" {
		t.Fatal("all-day shifted")
	}
	next := readCalendar(t, f.privateRequest(t, client, "GET", "/calendar?from=2026-10-13&to=2026-10-14", "", nil))
	if len(next.Items) != 0 {
		t.Fatal("exclusive end included")
	}
	private := readCalendar(t, f.privateRequest(t, other, "GET", "/calendar?from=2026-10-12&to=2026-10-13", "", nil))
	if len(private.Items) != 0 {
		t.Fatal("owner leak")
	}
	if r := f.privateRequest(t, other, "POST", "/calendar/events", "", in); r.status != 404 {
		t.Fatal("foreign task linked", r.status)
	}
	in = calendarInput("Неверный конец")
	in.End = in.Start
	if r := f.privateRequest(t, client, "POST", "/calendar/events", "", in); r.status != 422 {
		t.Fatal("empty range accepted")
	}
}
