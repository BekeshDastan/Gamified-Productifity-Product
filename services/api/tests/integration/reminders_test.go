//go:build integration

package integration

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/notifications"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func readReminder(t *testing.T, r authResponse) notifications.Reminder {
	t.Helper()
	var out struct{ Data notifications.Reminder }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func projectReminders(t *testing.T, f authFixture, s *notifications.Service, owner string) {
	t.Helper()
	if e := storage.WithinTx(f.ctx, f.pool, func(tx pgx.Tx) error { return s.ProjectReminders(f.ctx, tx, owner) }); e != nil {
		t.Fatal(e)
	}
}
func reminderFor(t *testing.T, f authFixture, owner, source string) notifications.Reminder {
	t.Helper()
	var id string
	if e := f.pool.QueryRow(f.ctx, `SELECT id::text FROM reminders WHERE owner_id=$1 AND source_id=$2 ORDER BY created_at DESC,source_revision DESC LIMIT 1`, owner, source).Scan(&id); e != nil {
		t.Fatal(e)
	}
	var r notifications.Reminder
	var raw []byte
	if e := f.pool.QueryRow(f.ctx, `SELECT row_to_json(v) FROM (SELECT id::text,source_kind,source_id::text,parent_id::text,source_revision,source_due_at,next_run_at,expires_at,version,state,urgent_override,snoozed_until,notification_id::text,source_url AS url,created_at,updated_at FROM reminders WHERE owner_id=$1 AND id=$2)v`, owner, id).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if e := json.Unmarshal(raw, &r); e != nil {
		t.Fatal(e)
	}
	return r
}
func TestReminderDeadlineDedupeSnoozeRevisionCancelAndLeases(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "reminder@example.test")
	b, _ := f.signedIn(t, "reminder-other@example.test")
	s := notifications.NewService(f.pool, f.clock).WithHabits(habits.NewService(f.pool, f.clock))
	input := questInput("PRIVATE reminder task")
	due := f.clock.Now().Add(90 * time.Minute)
	input.Deadline = &due
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	for i := 0; i < 3; i++ {
		projectReminders(t, f, s, who.ID)
	}
	r := reminderFor(t, f, who.ID, q.ID)
	if r.State != "scheduled" || r.SourceRevision != q.Version || !r.NextRunAt.Equal(due.Add(-time.Hour)) {
		t.Fatal(r)
	}
	store := jobs.Store{Pool: f.pool, Clock: f.clock}
	if _, e := store.Claim(f.ctx); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("early job", e)
	}
	var count int
	f.pool.QueryRow(f.ctx, `SELECT count(*) FROM reminders WHERE owner_id=$1`, who.ID).Scan(&count)
	if count != 1 {
		t.Fatal("reminder duplicated")
	}
	f.clock.Advance(31 * time.Minute)
	f.loginAs(t, a, "reminder@example.test")
	f.loginAs(t, b, "reminder-other@example.test")
	job, e := store.Claim(f.ctx)
	if e != nil || job.Kind != "notifications.reminder" {
		t.Fatal(job, e)
	}
	if e = store.Process(f.ctx, job, s.Handlers()); e != nil {
		t.Fatal(e)
	}
	if e = store.Process(f.ctx, job, s.Handlers()); e != jobs.ErrLeaseLost {
		t.Fatal("lease replay", e)
	}
	page := readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil))
	if len(page.Items) != 1 || page.Items[0].SourceID != q.ID {
		t.Fatal(page)
	}
	r = readReminder(t, f.privateRequest(t, a, "GET", "/reminders/"+r.ID, "", nil))
	if r.State != "fired" || r.Version != 2 {
		t.Fatal(r)
	}
	body := map[string]any{"minutes": 15, "urgent_override": true, "confirm": true}
	if res := f.privateRequest(t, b, "POST", "/reminders/"+r.ID+"/snooze", `"2"`, body); res.status != 404 {
		t.Fatal("foreign reminder", res.status)
	}
	r = readReminder(t, f.privateRequest(t, a, "POST", "/reminders/"+r.ID+"/snooze", `"2"`, body, "reminder-snooze-once-key"))
	again := readReminder(t, f.privateRequest(t, a, "POST", "/reminders/"+r.ID+"/snooze", `"2"`, body, "reminder-snooze-once-key"))
	if r.Version != 3 || again.Version != 3 || !r.UrgentOverride || !r.NextRunAt.Equal(f.clock.Now().Add(15*time.Minute)) {
		t.Fatal(r, again)
	}
	if readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil)).Unread != 0 {
		t.Fatal("old snoozed signal still active")
	}
	fresh := readQuest(t, f.privateRequest(t, a, "GET", "/quests/"+q.ID, "", nil))
	if fresh.Deadline == nil || !fresh.Deadline.Equal(due) || fresh.Version != q.Version {
		t.Fatal("snooze changed deadline")
	}
	f.clock.Advance(16 * time.Minute)
	f.loginAs(t, a, "reminder@example.test")
	job, e = store.Claim(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = store.Process(f.ctx, job, s.Handlers()); e != nil {
		t.Fatal(e)
	}
	page = readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil))
	if len(page.Items) != 2 || page.Unread != 1 {
		t.Fatal(page)
	}
	moved := due.Add(3 * time.Hour)
	edit := q.Input
	edit.Deadline = &moved
	q = readQuest(t, f.privateRequest(t, a, "PATCH", "/quests/"+q.ID, version(q), edit))
	projectReminders(t, f, s, who.ID)
	old := readReminder(t, f.privateRequest(t, a, "GET", "/reminders/"+r.ID, "", nil))
	if old.State != "cancelled" {
		t.Fatal("old revision live", old)
	}
	latest := reminderFor(t, f, who.ID, q.ID)
	if latest.ID == old.ID || latest.SourceRevision != q.Version {
		t.Fatal(latest)
	}
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]any{}))
	projectReminders(t, f, s, who.ID)
	f.clock.Advance(4 * time.Hour)
	f.loginAs(t, a, "reminder@example.test")
	job, e = store.Claim(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = store.Process(f.ctx, job, s.Handlers()); e != nil {
		t.Fatal(e)
	}
	page = readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil))
	if len(page.Items) != 2 || page.Unread != 0 {
		t.Fatal("completed source notified again", page)
	}
}
func TestReminderHabitCompletionCancelsPendingAndUrgentIsTaskOnly(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "habit-reminder@example.test")
	s := notifications.NewService(f.pool, f.clock).WithHabits(habits.NewService(f.pool, f.clock))
	h1 := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", habitInput()))
	in := habitInput()
	in.Title = "Second private habit"
	h2 := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", in))
	projectReminders(t, f, s, who.ID)
	r1 := reminderFor(t, f, who.ID, h1.Periods[0].ID)
	r2 := reminderFor(t, f, who.ID, h2.Periods[0].ID)
	readHabit(t, f.privateRequest(t, a, "POST", "/habits/"+h1.ID+"/logs", habitVersion(h1), habits.LogInput{PeriodID: h1.Periods[0].ID, Amount: 20, OccurredAt: f.clock.Now()}))
	f.clock.Advance(11*time.Hour + 31*time.Minute)
	f.loginAs(t, a, "habit-reminder@example.test")
	store := jobs.Store{Pool: f.pool, Clock: f.clock}
	for i := 0; i < 2; i++ {
		j, e := store.Claim(f.ctx)
		if e != nil {
			t.Fatal(e)
		}
		if e = store.Process(f.ctx, j, s.Handlers()); e != nil {
			t.Fatal(e)
		}
	}
	page := readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil))
	if len(page.Items) != 1 || page.Items[0].SourceID != r2.SourceID {
		t.Fatal("completed habit fired", page)
	}
	if readReminder(t, f.privateRequest(t, a, "GET", "/reminders/"+r1.ID, "", nil)).State != "cancelled" {
		t.Fatal("full norm reminder not cancelled")
	}
	r2 = readReminder(t, f.privateRequest(t, a, "GET", "/reminders/"+r2.ID, "", nil))
	for _, body := range []map[string]any{{"minutes": 5, "urgent_override": true, "confirm": true}, {"minutes": 60, "urgent_override": false, "confirm": true}, {"minutes": 5, "urgent_override": false, "confirm": false}} {
		if r := f.privateRequest(t, a, "POST", "/reminders/"+r2.ID+"/snooze", fmt.Sprintf(`"%d"`, r2.Version), body); r.status != 422 {
			t.Fatal(r.status, string(r.body))
		}
	}
}
func TestReminderSweepRealLeasesAndCrashRecovery(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "reminder-sweep@example.test")
	input := questInput("Soon")
	due := f.clock.Now().Add(30 * time.Minute)
	input.Deadline = &due
	readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	s := notifications.NewService(f.pool, f.clock).WithHabits(habits.NewService(f.pool, f.clock))
	if e := s.StartReminderSchedule(f.ctx); e != nil {
		t.Fatal(e)
	}
	store := jobs.Store{Pool: f.pool, Clock: f.clock}
	for _, kind := range []string{"notifications.reminders_sweep", "notifications.changed"} {
		j, e := store.Claim(f.ctx)
		if e != nil || j.Kind != kind {
			t.Fatal(j, e)
		}
		if e = store.Process(f.ctx, j, s.Handlers()); e != nil {
			t.Fatal(e)
		}
	}
	claimed, e := store.Claim(f.ctx)
	if e != nil || claimed.Kind != "notifications.reminder" {
		t.Fatal(claimed, e)
	}
	f.clock.Advance(31 * time.Second)
	replacement, e := store.Claim(f.ctx)
	if e != nil || replacement.ID != claimed.ID || replacement.LeaseToken == claimed.LeaseToken {
		t.Fatal("lease not recovered", replacement, e)
	}
	if e = store.Process(f.ctx, claimed, s.Handlers()); e != jobs.ErrLeaseLost {
		t.Fatal("old worker retained lease", e)
	}
	if e = store.Process(f.ctx, replacement, s.Handlers()); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM notifications WHERE owner_id=$1`, who.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
}
