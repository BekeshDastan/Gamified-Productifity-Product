//go:build integration

package integration

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/focus"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/notifications"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/push"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"github.com/jackc/pgx/v5"
	"strings"
	"sync"
	"testing"
	"time"
)

type notificationPush struct {
	err      error
	payloads []string
}
type blockingNotificationPush struct{ started, release chan struct{} }

func (s *blockingNotificationPush) Send(ctx context.Context, _ push.Subscription, _ []byte) error {
	close(s.started)
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func TestNotificationPushTransferWaitsForPreviousOwnerSend(t *testing.T) {
	sender := &blockingNotificationPush{make(chan struct{}), make(chan struct{})}
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(sender.release) }) }
	defer unblock()
	x := newPushReminder(t, sender)
	b, _ := x.f.signedIn(t, "push-transfer-race@example.test")
	var in notifications.PushInput
	in.Confirm = true
	if e := x.f.pool.QueryRow(x.f.ctx, `SELECT endpoint,p256dh,auth FROM notification_push_subscriptions WHERE owner_id=$1 AND id=$2`, x.who.ID, *x.delivery.SubscriptionID).Scan(&in.Endpoint, &in.P256dh, &in.Auth); e != nil {
		t.Fatal(e)
	}
	job, e := x.store.Claim(x.f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	sent := make(chan error, 1)
	go func() { sent <- x.store.Process(x.f.ctx, job, x.s.Handlers()) }()
	select {
	case <-sender.started:
	case <-time.After(5 * time.Second):
		t.Fatal("push did not start")
	}
	transferred := make(chan authResponse, 1)
	go func() { transferred <- x.f.privateRequest(t, b, "POST", "/notifications/push", `"0"`, in) }()
	select {
	case r := <-transferred:
		t.Fatal("transfer returned before prior send completed", r.status)
	case <-time.After(100 * time.Millisecond):
	}
	unblock()
	if e = <-sent; e != nil {
		t.Fatal(e)
	}
	r := <-transferred
	if r.status != 200 {
		t.Fatal("transfer failed", r.status)
	}
	var revoked bool
	if e = x.f.pool.QueryRow(x.f.ctx, `SELECT revoked_at IS NOT NULL FROM notification_push_subscriptions WHERE owner_id=$1 AND id=$2`, x.who.ID, *x.delivery.SubscriptionID).Scan(&revoked); e != nil || !revoked {
		t.Fatal("old subscription retained", e)
	}
}

func TestNotificationFocusSettlesWithoutOpenBrowser(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "focus-background@example.test")
	in := focusInput()
	session := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", in))
	s := notifications.NewService(f.pool, f.clock)
	fs := focus.NewService(f.pool, f.clock, nil).WithResultObserver(s.FocusChanged)
	s.WithFocusSettlement(fs.SettleDueInTx)
	f.clock.Advance(8 * time.Hour)
	var id string
	if e := f.pool.QueryRow(f.ctx, `INSERT INTO jobs(owner_id,kind,payload,dedupe_key,next_run_at,created_at) VALUES($1,'notifications.changed','{}',$2,$3,$3) RETURNING id::text`, who.ID, "focus-settle/"+session.ID, f.clock.Now()).Scan(&id); e != nil {
		t.Fatal(e)
	}
	store := jobs.Store{Pool: f.pool, Clock: f.clock}
	job, e := store.Claim(f.ctx)
	if e != nil || job.Kind != "notifications.changed" {
		t.Fatal(job, e)
	}
	if e = store.Process(f.ctx, job, s.Handlers()); e != nil {
		t.Fatal(e)
	}
	f.loginAs(t, a, who.Email)
	page := readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil))
	if len(page.Items) != 1 || page.Items[0].Kind != "focus_result" || page.Items[0].SourceID != session.ID {
		t.Fatal(page)
	}
	if e = store.Process(f.ctx, job, s.Handlers()); !errors.Is(e, jobs.ErrLeaseLost) {
		t.Fatal("replayed lease", e)
	}
	var state string
	if e = f.pool.QueryRow(f.ctx, `SELECT state FROM focus_sessions WHERE owner_id=$1 AND id=$2`, who.ID, session.ID).Scan(&state); e != nil || state != "awaiting_confirmation" {
		t.Fatal(state, e)
	}
}

func (s *notificationPush) Send(_ context.Context, _ push.Subscription, body []byte) error {
	s.payloads = append(s.payloads, string(body))
	return s.err
}
func newPushReminder(t *testing.T, sender push.Sender) emailReminderFixture {
	t.Helper()
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "push-delivery-"+testkit.NewUser().ID+"@example.test")
	p := notifications.DefaultPreferences()
	p.Quiet.Enabled = false
	p.Channels[0].Push = true
	readNotificationPrefs(t, f.privateRequest(t, a, "PUT", "/notifications/preferences", `"0"`, p))
	key, e := ecdh.P256().GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	in := notifications.PushInput{Endpoint: "https://fcm.googleapis.com/fcm/send/test", P256dh: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), Auth: base64.RawURLEncoding.EncodeToString(make([]byte, 16)), Confirm: true}
	if r := f.privateRequest(t, a, "POST", "/notifications/push", `"0"`, in); r.status != 200 {
		t.Fatal(r.status, string(r.body))
	}
	s := notifications.NewService(f.pool, f.clock).WithHabits(habits.NewService(f.pool, f.clock))
	s.ConfigurePush("test-public-key", sender)
	input := questInput("PRIVATE_PUSH_TASK_DIARY")
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
	ds := readDeliveries(t, f.privateRequest(t, a, "GET", "/notifications/"+page.Items[0].ID+"/deliveries", "", nil))
	if len(ds) != 1 || ds[0].Channel != "push" {
		t.Fatal(ds)
	}
	return emailReminderFixture{f, a, who, s, store, page.Items[0].ID, ds[0]}
}
func TestNotificationPushDeliveryOutcomesWithdrawalAndExpiry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		state string
	}{{"accepted", nil, "provider_accepted"}, {"gone", push.ErrGone, "failed"}, {"unknown", push.ErrUnknown, "delivery_unknown"}, {"rejected", push.ErrRejected, "failed"}, {"unavailable", push.ErrUnavailable, "failed"}} {
		t.Run(tc.name, func(t *testing.T) {
			sender := &notificationPush{err: tc.err}
			x := newPushReminder(t, sender)
			d := x.process(t)
			if d.State != tc.state || len(sender.payloads) != 1 || strings.Contains(sender.payloads[0], "PRIVATE_") {
				t.Fatal(d, sender.payloads)
			}
			if _, e := x.store.Claim(x.f.ctx); !errors.Is(e, pgx.ErrNoRows) {
				t.Fatal("unexpected auto retry", e)
			}
			if errors.Is(tc.err, push.ErrGone) {
				var revoked bool
				if e := x.f.pool.QueryRow(x.f.ctx, `SELECT revoked_at IS NOT NULL FROM notification_push_subscriptions WHERE owner_id=$1 AND id=$2`, x.who.ID, *d.SubscriptionID).Scan(&revoked); e != nil || !revoked {
					t.Fatal("provider 410 not revoked", e)
				}
			}
		})
	}
	for _, which := range []string{"consent", "subscription", "expiry"} {
		t.Run(which, func(t *testing.T) {
			sender := &notificationPush{}
			x := newPushReminder(t, sender)
			if which == "consent" {
				p := readNotificationPrefs(t, x.f.privateRequest(t, x.a, "GET", "/notifications/preferences", "", nil))
				p.Channels[0].Push = false
				readNotificationPrefs(t, x.f.privateRequest(t, x.a, "PUT", "/notifications/preferences", fmt.Sprintf(`"%d"`, p.Version), p.PreferencesInput))
			} else {
				sql := `UPDATE notification_push_subscriptions SET revoked_at=$3 WHERE owner_id=$1 AND id=$2`
				if which == "expiry" {
					sql = `UPDATE notification_push_subscriptions SET expires_at=$3 WHERE owner_id=$1 AND id=$2`
				}
				if _, e := x.f.pool.Exec(x.f.ctx, sql, x.who.ID, *x.delivery.SubscriptionID, x.f.clock.Now()); e != nil {
					t.Fatal(e)
				}
			}
			d := x.process(t)
			if d.State != "cancelled" || len(sender.payloads) != 0 {
				t.Fatal("push sent after withdrawal/expiry", d)
			}
			if len(readNotifications(t, x.f.privateRequest(t, x.a, "GET", "/notifications", "", nil)).Items) != 1 {
				t.Fatal("inapp lost")
			}
		})
	}
}
func TestNotificationDeliveryQuietAcrossMidnightAndRollingCap(t *testing.T) {
	sender := &notificationMailer{}
	x := newEmailReminder(t, sender)
	p := readNotificationPrefs(t, x.f.privateRequest(t, x.a, "GET", "/notifications/preferences", "", nil))
	p.Quiet = notifications.QuietWindow{Enabled: true, Start: "23:00", End: "13:00"}
	p.DailyCap = 1
	readNotificationPrefs(t, x.f.privateRequest(t, x.a, "PUT", "/notifications/preferences", fmt.Sprintf(`"%d"`, p.Version), p.PreferencesInput))
	d := x.process(t)
	if d.State != "queued" || d.Attempts != 0 || len(sender.snapshot()) != 0 || d.NextRunAt.UTC().Hour() != 13 {
		t.Fatal("quiet not deferred", d)
	}
	x.f.clock.Advance(d.NextRunAt.Sub(x.f.clock.Now()))
	x.f.loginAs(t, x.a, x.who.Email)
	d = x.process(t)
	if d.State != "provider_accepted" || len(sender.snapshot()) != 1 {
		t.Fatal(d)
	}
	input := questInput("SECOND_PRIVATE_SOURCE")
	due := x.f.clock.Now().Add(30 * time.Minute)
	input.Deadline = &due
	q := readQuest(t, x.f.privateRequest(t, x.a, "POST", "/quests", "", input))
	projectReminders(t, x.f, x.s, x.who.ID)
	job, e := x.store.Claim(x.f.ctx)
	if e != nil || job.Kind != "notifications.reminder" {
		t.Fatal(job, e)
	}
	if e = x.store.Process(x.f.ctx, job, x.s.Handlers()); e != nil {
		t.Fatal(e)
	}
	page := readNotifications(t, x.f.privateRequest(t, x.a, "GET", "/notifications", "", nil))
	found := false
	for _, n := range page.Items {
		if n.SourceID == q.ID {
			ds := readDeliveries(t, x.f.privateRequest(t, x.a, "GET", "/notifications/"+n.ID+"/deliveries", "", nil))
			if len(ds) != 1 || !ds[0].NextRunAt.After(x.f.clock.Now()) || ds[0].Attempts != 0 {
				raw, _ := json.Marshal(ds)
				t.Fatal("cap not shared", string(raw))
			}
			found = true
		}
	}
	if !found || len(sender.snapshot()) != 1 {
		t.Fatal("missing capped event")
	}
}
