//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/notifications"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/mail"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

type notificationMailer struct {
	mu       sync.Mutex
	messages []mail.Message
	err      error
	after    func()
}

func (m *notificationMailer) Send(_ context.Context, message mail.Message) error {
	m.mu.Lock()
	m.messages = append(m.messages, message)
	err, after := m.err, m.after
	m.mu.Unlock()
	if after != nil {
		after()
	}
	return err
}
func (m *notificationMailer) snapshot() []mail.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mail.Message{}, m.messages...)
}

type emailReminderFixture struct {
	f              authFixture
	a              *http.Client
	who            auth.Account
	s              *notifications.Service
	store          jobs.Store
	notificationID string
	delivery       notifications.Delivery
}

func readDeliveries(t *testing.T, r authResponse) []notifications.Delivery {
	t.Helper()
	var out struct {
		Data struct{ Items []notifications.Delivery }
	}
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data.Items
}
func newEmailReminder(t *testing.T, sender mail.Mailer) emailReminderFixture {
	t.Helper()
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "notification-mail-"+testkit.NewUser().ID+"@example.test")
	if _, e := f.pool.Exec(f.ctx, `UPDATE users SET email_verified_at=$2 WHERE id=$1`, who.ID, f.clock.Now()); e != nil {
		t.Fatal(e)
	}
	p := notifications.DefaultPreferences()
	p.Quiet.Enabled = false
	p.Channels[0].Email = true
	readNotificationPrefs(t, f.privateRequest(t, a, "PUT", "/notifications/preferences", `"0"`, p))
	s := notifications.NewService(f.pool, f.clock).WithHabits(habits.NewService(f.pool, f.clock))
	if e := s.ConfigureDelivery("http://localhost:3000", bytes.Repeat([]byte{12}, 32), sender); e != nil {
		t.Fatal(e)
	}
	input := questInput("PRIVATE_TASK_NOT_IN_EMAIL")
	due := f.clock.Now().Add(30 * time.Minute)
	input.Deadline = &due
	readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	projectReminders(t, f, s, who.ID)
	store := jobs.Store{Pool: f.pool, Clock: f.clock}
	job, e := store.Claim(f.ctx)
	if e != nil || job.Kind != "notifications.reminder" {
		t.Fatal(job, e)
	}
	if e = store.Process(f.ctx, job, s.Handlers()); e != nil {
		t.Fatal(e)
	}
	page := readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil))
	if len(page.Items) != 1 {
		t.Fatal(page)
	}
	list := readDeliveries(t, f.privateRequest(t, a, "GET", "/notifications/"+page.Items[0].ID+"/deliveries", "", nil))
	if len(list) != 1 || list[0].State != "queued" {
		t.Fatal(list)
	}
	return emailReminderFixture{f, a, who, s, store, page.Items[0].ID, list[0]}
}
func (x emailReminderFixture) process(t *testing.T) notifications.Delivery {
	t.Helper()
	job, e := x.store.Claim(x.f.ctx)
	if e != nil || job.Kind != "notifications.delivery" {
		t.Fatal(job, e)
	}
	if e = x.store.Process(x.f.ctx, job, x.s.Handlers()); e != nil {
		t.Fatal(e)
	}
	return readDeliveries(t, x.f.privateRequest(t, x.a, "GET", "/notifications/"+x.notificationID+"/deliveries", "", nil))[0]
}
func TestNotificationEmailAcceptedPrivacyAndConfirmedUnsubscribe(t *testing.T) {
	sender := &notificationMailer{}
	x := newEmailReminder(t, sender)
	d := x.process(t)
	if d.State != "provider_accepted" || d.ProviderAcceptedAt == nil || d.Attempts != 1 {
		t.Fatal(d)
	}
	messages := sender.snapshot()
	if len(messages) != 1 || messages[0].To != x.who.Email || strings.Contains(messages[0].Text, "PRIVATE_TASK") || !strings.Contains(messages[0].Text, "/app/notifications") {
		t.Fatal("email privacy/template")
	}
	parts := strings.Split(messages[0].Text, "#token=")
	if len(parts) != 2 {
		t.Fatal("missing unsubscribe link")
	}
	token := strings.TrimSpace(parts[1])
	before := readNotificationPrefs(t, x.f.privateRequest(t, x.a, "GET", "/notifications/preferences", "", nil))
	if r := x.f.privateRequest(t, x.a, "POST", "/notifications/unsubscribe/preview", "", map[string]string{"token": token}); r.status != 200 {
		t.Fatal(r.status, string(r.body))
	}
	if !readNotificationPrefs(t, x.f.privateRequest(t, x.a, "GET", "/notifications/preferences", "", nil)).Channel("deadline").Email {
		t.Fatal("preview unsubscribed")
	}
	if r := x.f.privateRequest(t, x.a, "POST", "/notifications/unsubscribe", "", map[string]any{"token": token, "confirm": false}); r.status != 422 {
		t.Fatal("consent omitted")
	}
	for i := 0; i < 2; i++ {
		if r := x.f.privateRequest(t, x.a, "POST", "/notifications/unsubscribe", "", map[string]any{"token": token, "confirm": true}); r.status != 200 {
			t.Fatal(r.status, string(r.body))
		}
	}
	after := readNotificationPrefs(t, x.f.privateRequest(t, x.a, "GET", "/notifications/preferences", "", nil))
	if after.Channel("deadline").Email || after.Version != before.Version+1 {
		t.Fatal(after)
	}
	if len(readNotifications(t, x.f.privateRequest(t, x.a, "GET", "/notifications", "", nil)).Items) != 1 {
		t.Fatal("inbox disappeared")
	}
}
func TestNotificationEmailUnknownNeverRetriesWithoutAcknowledgement(t *testing.T) {
	sender := &notificationMailer{err: mail.ErrDeliveryUnknown}
	x := newEmailReminder(t, sender)
	d := x.process(t)
	if d.State != "delivery_unknown" || d.UnknownAt == nil || d.ProviderAcceptedAt != nil {
		t.Fatal(d)
	}
	if _, e := x.store.Claim(x.f.ctx); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("unknown auto-retried", e)
	}
	path := "/notification-deliveries/" + d.ID + "/retry"
	body := map[string]bool{"confirm": true, "acknowledge_duplicates": false}
	if r := x.f.privateRequest(t, x.a, "POST", path, fmt.Sprintf(`"%d"`, d.Version), body); r.status != 422 {
		t.Fatal("risk not acknowledged")
	}
	body["acknowledge_duplicates"] = true
	if r := x.f.privateRequest(t, x.a, "POST", path, fmt.Sprintf(`"%d"`, d.Version), body, "notification-manual-retry-key"); r.status != 200 {
		t.Fatal(r.status, string(r.body))
	}
	sender.err = nil
	d = x.process(t)
	if d.State != "provider_accepted" || d.Attempts != 2 || len(sender.snapshot()) != 2 {
		t.Fatal(d)
	}
	var n int
	x.f.pool.QueryRow(x.f.ctx, `SELECT count(*) FROM notification_delivery_attempts WHERE owner_id=$1 AND state='delivery_unknown'`, x.who.ID).Scan(&n)
	if n != 1 {
		t.Fatal("unknown evidence erased")
	}
}
func TestNotificationEmailSafeFailureRetriesThreeTimesAndOptOutCancels(t *testing.T) {
	sender := &notificationMailer{err: &textproto.Error{Code: 450, Msg: "temporary rejection"}}
	x := newEmailReminder(t, sender)
	for i, delay := range []time.Duration{time.Minute, 2 * time.Minute, 0} {
		d := x.process(t)
		if i < 2 && d.State != "retry_wait" || i == 2 && d.State != "failed" {
			t.Fatal(d)
		}
		x.f.clock.Advance(delay)
	}
	if len(sender.snapshot()) != 3 {
		t.Fatal("retry cap")
	}
	if _, e := x.store.Claim(x.f.ctx); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("unbounded retries")
	}
	sender2 := &notificationMailer{}
	other := newEmailReminder(t, sender2)
	p := readNotificationPrefs(t, other.f.privateRequest(t, other.a, "GET", "/notifications/preferences", "", nil))
	input := p.PreferencesInput
	input.Channels[0].Email = false
	readNotificationPrefs(t, other.f.privateRequest(t, other.a, "PUT", "/notifications/preferences", `"1"`, input))
	d := other.process(t)
	if d.State != "cancelled" || d.Attempts != 0 || len(sender2.snapshot()) != 0 {
		t.Fatal("withdrawn consent sent", d)
	}
	if readNotifications(t, other.f.privateRequest(t, other.a, "GET", "/notifications", "", nil)).Unread != 1 {
		t.Fatal("SMTP cancellation affected inbox")
	}
}
func TestNotificationEmailCrashAfterAcceptanceIsDurablyUnknown(t *testing.T) {
	sender := &notificationMailer{}
	x := newEmailReminder(t, sender)
	ctx, cancel := context.WithCancel(x.f.ctx)
	sender.after = cancel
	job, e := x.store.Claim(x.f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = x.store.Process(ctx, job, x.s.Handlers()); e == nil {
		t.Fatal("crash did not interrupt outcome commit")
	}
	x.f.clock.Advance(time.Minute)
	sender.after = nil
	replacement, e := x.store.Claim(x.f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = x.store.Process(x.f.ctx, replacement, x.s.Handlers()); e != nil {
		t.Fatal(e)
	}
	d := readDeliveries(t, x.f.privateRequest(t, x.a, "GET", "/notifications/"+x.notificationID+"/deliveries", "", nil))[0]
	if d.State != "delivery_unknown" || len(sender.snapshot()) != 1 {
		t.Fatal("crash resent message", d)
	}
	if len(readNotifications(t, x.f.privateRequest(t, x.a, "GET", "/notifications", "", nil)).Items) != 1 {
		t.Fatal("inbox rollback")
	}
}
