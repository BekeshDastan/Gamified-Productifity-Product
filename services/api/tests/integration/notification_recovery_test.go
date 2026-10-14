//go:build integration

package integration

import (
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/notifications"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"testing"
	"time"
)

func processNotificationChange(t *testing.T, x emailReminderFixture) {
	t.Helper()
	if _, e := x.f.pool.Exec(x.f.ctx, `INSERT INTO jobs(owner_id,kind,payload,dedupe_key,next_run_at,created_at) VALUES($1,'notifications.changed','{}',$2,$3,$3)`, x.who.ID, "notification-repair/"+testkit.NewUser().ID, x.f.clock.Now()); e != nil {
		t.Fatal(e)
	}
	j, e := x.store.Claim(x.f.ctx)
	if e != nil || j.Kind != "notifications.changed" {
		t.Fatal(j, e)
	}
	if e = x.store.Process(x.f.ctx, j, x.s.Handlers()); e != nil {
		t.Fatal(e)
	}
}
func TestNotificationQueueRepairAndRetryOwnership(t *testing.T) {
	sender := &notificationMailer{}
	x := newEmailReminder(t, sender)
	if _, e := x.f.pool.Exec(x.f.ctx, `UPDATE jobs SET state='dead' WHERE owner_id=$1 AND kind='notifications.delivery'`, x.who.ID); e != nil {
		t.Fatal(e)
	}
	processNotificationChange(t, x)
	d := x.process(t)
	if d.State != "provider_accepted" || len(sender.snapshot()) != 1 || d.Attempts != 1 {
		t.Fatal("dead queue not repaired", d)
	}
	b, _ := x.f.signedIn(t, "foreign-retry@example.test")
	body := map[string]bool{"confirm": true, "acknowledge_duplicates": true}
	for _, id := range []string{"invalid", testkit.NewUser().ID} {
		if r := x.f.privateRequest(t, x.a, "POST", "/notification-deliveries/"+id+"/retry", `"1"`, body); r.status != 404 {
			t.Fatal("missing retry", r.status)
		}
	}
	if r := x.f.privateRequest(t, b, "POST", "/notification-deliveries/"+d.ID+"/retry", fmt.Sprintf(`"%d"`, d.Version), body); r.status != 404 {
		t.Fatal("foreign retry", r.status)
	}
	if r := x.f.privateRequest(t, x.a, "POST", "/notification-deliveries/"+d.ID+"/retry", fmt.Sprintf(`"%d"`, d.Version), body); r.status != 409 {
		t.Fatal("accepted retry", r.status)
	}
}
func TestNotificationRepairInterruptedSendingWithoutResend(t *testing.T) {
	sender := &notificationMailer{}
	x := newEmailReminder(t, sender)
	attempt := testkit.NewUser().ID
	now := x.f.clock.Now()
	if _, e := x.f.pool.Exec(x.f.ctx, `UPDATE notification_deliveries SET state='sending',version=version+1,attempts=1,auto_attempts=1,last_attempt_id=$3 WHERE owner_id=$1 AND id=$2`, x.who.ID, x.delivery.ID, attempt); e != nil {
		t.Fatal(e)
	}
	if _, e := x.f.pool.Exec(x.f.ctx, `INSERT INTO notification_delivery_attempts(owner_id,delivery_id,attempt,attempt_id,channel,state,started_at) VALUES($1,$2,1,$3,'email','sending',$4)`, x.who.ID, x.delivery.ID, attempt, now); e != nil {
		t.Fatal(e)
	}
	if _, e := x.f.pool.Exec(x.f.ctx, `UPDATE jobs SET state='dead' WHERE owner_id=$1 AND kind='notifications.delivery'`, x.who.ID); e != nil {
		t.Fatal(e)
	}
	x.f.clock.Advance(3 * time.Minute)
	processNotificationChange(t, x)
	d := readDeliveries(t, x.f.privateRequest(t, x.a, "GET", "/notifications/"+x.notificationID+"/deliveries", "", nil))[0]
	if d.State != "delivery_unknown" || len(sender.snapshot()) != 0 || d.Attempts != 1 {
		t.Fatal("interrupted attempt resent", d)
	}
}
func TestNotificationReminderContinuationKeepsAll501Sources(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "reminder-pages@example.test")
	input := questInput("Bounded reminder fixture")
	due := f.clock.Now().Add(2 * time.Hour)
	input.Deadline = &due
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	if _, e := f.pool.Exec(f.ctx, `INSERT INTO quests(owner_id,area_id,payload,status,created_at,updated_at,deadline) SELECT owner_id,area_id,payload,status,created_at,updated_at,deadline FROM quests CROSS JOIN generate_series(1,500) WHERE id=$1`, q.ID); e != nil {
		t.Fatal(e)
	}
	s := notifications.NewService(f.pool, f.clock)
	projectReminders(t, f, s, who.ID)
	var n int
	if e := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM reminders WHERE owner_id=$1`, who.ID).Scan(&n); e != nil || n != 500 {
		t.Fatal("unbounded first projection", n, e)
	}
	store := jobs.Store{Pool: f.pool, Clock: f.clock}
	j, e := store.Claim(f.ctx)
	if e != nil || j.Kind != "notifications.reminders_continue" {
		t.Fatal(j, e)
	}
	if e = store.Process(f.ctx, j, s.Handlers()); e != nil {
		t.Fatal(e)
	}
	projectReminders(t, f, s, who.ID)
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM reminders WHERE owner_id=$1`, who.ID).Scan(&n); e != nil || n != 501 {
		t.Fatal("continuation lost or duplicated source", n, e)
	}
	if r := f.privateRequest(t, a, "GET", "/reminders", "", nil); r.status != 200 {
		t.Fatal(r.status, string(r.body))
	}
	// A second change in the same clock minute must create a fresh continuation,
	// even though the previous chain's job already succeeded.
	p := notifications.DefaultPreferences()
	p.DeadlineLeadMinutes = 30
	readNotificationPrefs(t, f.privateRequest(t, a, "PUT", "/notifications/preferences", `"0"`, p))
	projectReminders(t, f, s, who.ID)
	j, e = store.Claim(f.ctx)
	if e != nil || j.Kind != "notifications.reminders_continue" {
		t.Fatal("same-minute continuation suppressed", j, e)
	}
	if e = store.Process(f.ctx, j, s.Handlers()); e != nil {
		t.Fatal(e)
	}
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM reminders WHERE owner_id=$1 AND next_run_at=$2`, who.ID, due.Add(-30*time.Minute)).Scan(&n); e != nil || n != 501 {
		t.Fatal("preference change missed tail source", n, e)
	}
}
