//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"strings"
	"testing"
	"time"
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
func TestCalendarConcurrentMaterializationAndRollback(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "calendar-atomic@example.test")
	in := calendar.SeriesInput{Event: calendarInput("Атомарное повторение"), Kind: "task", Rule: calendar.Rule{Frequency: "daily", Interval: 1, Count: 2, Weekdays: []int{}}}
	readSeries(t, f.privateRequest(t, client, "POST", "/calendar/series", "", in))
	_, e := f.pool.Exec(t.Context(), `CREATE FUNCTION reject_calendar_history() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced_calendar_history_failure'; END $$; CREATE TRIGGER reject_calendar_history BEFORE INSERT ON calendar_event_history FOR EACH ROW EXECUTE FUNCTION reject_calendar_history()`)
	if e != nil {
		t.Fatal(e)
	}
	path := "/calendar?from=2026-10-12&to=2026-10-15"
	if r := f.privateRequest(t, client, "GET", path, "", nil); r.status != 500 {
		t.Fatal("forced failure missing")
	}
	var count int
	if e := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM quests`).Scan(&count); e != nil || count != 0 {
		t.Fatal("orphan task after rollback", count, e)
	}
	if _, e := f.pool.Exec(t.Context(), `DROP TRIGGER reject_calendar_history ON calendar_event_history`); e != nil {
		t.Fatal(e)
	}
	results := make(chan authResponse, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- f.privateRequest(t, client, "GET", path, "", nil) }()
	}
	for i := 0; i < 2; i++ {
		page := readCalendar(t, <-results)
		if len(page.Items) != 4 {
			t.Fatal("concurrent range", len(page.Items))
		}
	}
	if e := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM quests`).Scan(&count); e != nil || count != 2 {
		t.Fatal("concurrent duplicated task", count, e)
	}
	event := readEvent(t, f.privateRequest(t, client, "POST", "/calendar/events", "", calendarInput("Конкурирующее изменение")))
	edit := calendar.Edit{Input: event.Input, Reason: "Изменение из двух окон"}
	edit.Start = "2026-10-16"
	edit.End = "2026-10-17"
	for i := 0; i < 2; i++ {
		go func() {
			results <- f.privateRequest(t, client, "PATCH", "/calendar/events/"+event.ID, eventVersion(event), edit)
		}()
	}
	a, b := <-results, <-results
	if !((a.status == 200 && b.status == 409) || (a.status == 409 && b.status == 200)) {
		t.Fatal("lost update", a.status, b.status)
	}
}
func TestCalendarTimezoneChangeAndClockHorizon(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "calendar-zone@example.test")
	in := calendar.SeriesInput{Event: calendar.Input{Title: "Серия в своём поясе", Start: "2026-10-12T23:30:00-12:00", End: "2026-10-13T00:30:00-12:00", Timezone: "Etc/GMT+12"}, Kind: "event", Rule: calendar.Rule{Frequency: "daily", Interval: 1, Count: 65, Weekdays: []int{}}}
	series := readSeries(t, f.privateRequest(t, client, "POST", "/calendar/series", "", in))
	whole := calendarInput("Без времени")
	readEvent(t, f.privateRequest(t, client, "POST", "/calendar/events", "", whole))
	var stable string
	for _, zone := range []string{"Pacific/Kiritimati", "UTC", "Asia/Almaty", "America/New_York"} {
		if _, e := f.pool.Exec(t.Context(), `UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}',to_jsonb($2::text)) WHERE owner_id=$1`, account.ID, zone); e != nil {
			t.Fatal(e)
		}
		page := readCalendar(t, f.privateRequest(t, client, "GET", "/calendar?from=2026-10-12&to=2026-10-15", "", nil))
		found := false
		for _, v := range page.Items {
			if v.Title == whole.Title && v.Start != whole.Start {
				t.Fatal("all day shifted")
			}
			if v.SeriesID != nil && *v.SeriesID == series.ID && *v.OccurrenceKey == "2026-10-12" {
				found = true
				if stable == "" {
					stable = v.ID
				}
				if v.ID != stable || v.Start != "2026-10-13T11:30:00Z" {
					t.Fatal("zone changed identity or instant", zone, v)
				}
			}
		}
		if !found {
			t.Fatal("cross-zone instance omitted", zone)
		}
	}
	svc := calendar.NewService(f.pool, f.clock, quests.NewService(f.pool, f.clock))
	if e := svc.FillHorizon(t.Context()); e != nil {
		t.Fatal(e)
	}
	var before int
	if e := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM calendar_events WHERE series_id=$1`, series.ID).Scan(&before); e != nil {
		t.Fatal(e)
	}
	f.clock.Advance(24 * time.Hour)
	if e := svc.FillHorizon(t.Context()); e != nil {
		t.Fatal(e)
	}
	var after int
	if e := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM calendar_events WHERE series_id=$1`, series.ID).Scan(&after); e != nil {
		t.Fatal(e)
	}
	if after != before+1 {
		t.Fatal("clock horizon", before, after)
	}
}
func TestCalendarOriginalPlanAndDeadlineStaySeparate(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "calendar-outcomes@example.test")
	in := questInput("Результат с дедлайном")
	planned := "2026-10-10"
	in.PlannedDate = &planned
	deadline := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	in.Deadline = &deadline
	q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", in))
	q = readQuest(t, f.privateRequest(t, client, "PATCH", "/calendar/quests/"+q.ID, version(q), map[string]any{"planned_date": "2026-10-12", "reason": "Не успел в исходный день"}))
	q = readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]any{}))
	page := readCalendar(t, f.privateRequest(t, client, "GET", "/calendar?from=2026-10-10&to=2026-10-13", "", nil))
	history, limit, actual := false, false, false
	for _, v := range page.Items {
		switch v.Source {
		case "history":
			history = v.Start == planned && v.Status == "postponed" && v.Reason == "Не успел в исходный день"
		case "deadline":
			limit = v.Start == deadline.Format(time.RFC3339) && strings.Contains(v.Reason, "позже дедлайна")
		case "quest":
			actual = v.Start == "2026-10-12" && v.Status == "completed"
		}
	}
	if !history || !limit || !actual {
		t.Fatalf("misleading outcome: history=%v deadline=%v actual=%v", history, limit, actual)
	}
}
func readSeries(t *testing.T, r authResponse) calendar.Series {
	t.Helper()
	var v struct {
		Data calendar.Series `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &v) != nil || v.Data.ID == "" {
		t.Fatalf("series %d %s", r.status, r.body)
	}
	return v.Data
}
func TestCalendarExceptionsSplitEndAndCompletedHistory(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "calendar-exceptions@example.test")
	other, _ := f.signedIn(t, "foreign-series@example.test")
	in := calendar.SeriesInput{Event: calendarInput("Серия задач"), Kind: "task", Rule: calendar.Rule{Frequency: "daily", Interval: 1, Count: 5, Weekdays: []int{}}}
	series := readSeries(t, f.privateRequest(t, client, "POST", "/calendar/series", "", in))
	path := "/calendar?from=2026-10-12&to=2026-10-21"
	page := readCalendar(t, f.privateRequest(t, client, "GET", path, "", nil))
	byDate := map[string]calendar.Event{}
	for _, v := range page.Items {
		if v.SeriesID != nil {
			byDate[*v.OccurrenceKey] = v
		}
	}
	done := byDate["2026-10-14"]
	q := readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+*done.QuestID, "", nil))
	q = readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]any{}))
	completedVersion := q.Version
	cancel := byDate["2026-10-15"]
	cancel = readEvent(t, f.privateRequest(t, client, "PATCH", "/calendar/events/"+cancel.ID, eventVersion(cancel), calendar.Edit{Input: cancel.Input, Reason: "Отдых", QuestVersion: cancel.QuestVersion, Cancelled: true}))
	if !cancel.Cancelled || cancel.Status != "skipped" {
		t.Fatal("cancel not reflected in task")
	}
	replacement := in
	replacement.Event.Start = "2026-10-16"
	replacement.Event.End = "2026-10-17"
	replacement.Event.Title = "Продолжение"
	replacement.Rule.Count = 4
	edit := calendar.SeriesEdit{Action: "split", OccurrenceKey: "2026-10-13", Replacement: &replacement, Reason: "Меняю будущий план"}
	key := "calendar-split-replay-key"
	next := readSeries(t, f.privateRequest(t, client, "PATCH", "/calendar/series/"+series.ID, fmt.Sprintf(`"%d"`, series.Version), edit, key))
	replay := readSeries(t, f.privateRequest(t, client, "PATCH", "/calendar/series/"+series.ID, fmt.Sprintf(`"%d"`, series.Version), edit, key))
	if next.ID == series.ID || next.ID != replay.ID {
		t.Fatal("split replay")
	}
	for i := 0; i < 2; i++ {
		page = readCalendar(t, f.privateRequest(t, client, "GET", path, "", nil))
	}
	active := 0
	for _, v := range page.Items {
		if v.SeriesID != nil && *v.SeriesID == next.ID && !v.Cancelled {
			active++
			if *v.OccurrenceKey == "2026-10-17" || *v.OccurrenceKey == "2026-10-18" {
				t.Fatal("history resurrected")
			}
		}
	}
	if active != 2 {
		t.Fatal("future instances", active)
	}
	q = readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+q.ID, "", nil))
	if q.Version != completedVersion || q.Status != "completed" || *q.PlannedDate != "2026-10-14" {
		t.Fatal("completion rewritten")
	}
	until := "2026-10-16"
	next = readSeries(t, f.privateRequest(t, client, "PATCH", "/calendar/series/"+next.ID, fmt.Sprintf(`"%d"`, next.Version), calendar.SeriesEdit{Action: "end", Until: &until, Reason: "Сокращаю серию"}))
	page = readCalendar(t, f.privateRequest(t, client, "GET", path, "", nil))
	for _, v := range page.Items {
		if v.SeriesID != nil && *v.SeriesID == next.ID && *v.OccurrenceKey > until && !v.Cancelled {
			t.Fatal("end did not cancel future")
		}
	}
	if r := f.privateRequest(t, other, "GET", "/calendar/series/"+next.ID, "", nil); r.status != 404 {
		t.Fatal("series ownership")
	}
	if r := f.privateRequest(t, client, "PATCH", "/calendar/series/"+next.ID, `"1"`, calendar.SeriesEdit{Action: "end", Until: &until, Reason: "Старая версия"}); r.status != 409 {
		t.Fatal("stale series")
	}
	svc := calendar.NewService(f.pool, f.clock, quests.NewService(f.pool, f.clock))
	if e := svc.FillHorizon(t.Context()); e != nil {
		t.Fatal(e)
	}
	page = readCalendar(t, f.privateRequest(t, client, "GET", path, "", nil))
	for _, v := range page.Items {
		if v.ID == cancel.ID && !v.Cancelled {
			t.Fatal("cancel lost after worker restart")
		}
	}
}
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
