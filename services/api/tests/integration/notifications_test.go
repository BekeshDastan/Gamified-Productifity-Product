//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/notifications"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
	"time"
)

func readNotifications(t *testing.T, r authResponse) notifications.Page {
	t.Helper()
	var out struct{ Data notifications.Page }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func readNotification(t *testing.T, r authResponse) notifications.Notification {
	t.Helper()
	var out struct{ Data notifications.Notification }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestNotificationsDedupePaginationReadStateAndOwner(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "notifications@example.test")
	b, other := f.signedIn(t, "notifications-other@example.test")
	service := notifications.NewService(f.pool, f.clock)
	source := testkit.NewUser().ID
	event := notifications.Event{Kind: "deadline", SourceKind: "task", SourceID: source, SourceRevision: 1, EventKey: "deadline/" + source, URL: "/app/quests/" + source}
	emitRaw := func(owner string, event notifications.Event) (notifications.Notification, error) {
		var result notifications.Notification
		e := storage.WithinTx(f.ctx, f.pool, func(tx pgx.Tx) error {
			var locked string
			if e := tx.QueryRow(f.ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, owner).Scan(&locked); e != nil {
				return e
			}
			var e error
			result, _, e = service.EmitInTx(f.ctx, tx, owner, event)
			return e
		})
		return result, e
	}
	emit := func(owner string, event notifications.Event) notifications.Notification {
		t.Helper()
		n, e := emitRaw(owner, event)
		if e != nil {
			t.Fatal(e)
		}
		return n
	}
	out := make([]notifications.Notification, 4)
	errs := make([]error, 4)
	var wg sync.WaitGroup
	for i := range out {
		wg.Add(1)
		go func(i int) { defer wg.Done(); out[i], errs[i] = emitRaw(who.ID, event) }(i)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	for _, n := range out {
		if n.ID != out[0].ID {
			t.Fatal("duplicate notification")
		}
	}
	for i := 0; i < 25; i++ {
		id := testkit.NewUser().ID
		emit(who.ID, notifications.Event{Kind: "report", SourceKind: "report", SourceID: id, SourceRevision: 1, EventKey: "report/" + id, URL: "/app/analytics?report_id=" + id})
	}
	foreign := emit(other.ID, event)
	page := readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil))
	if len(page.Items) != 25 || page.Next == "" || page.Unread != 26 {
		t.Fatal(page)
	}
	last := readNotifications(t, f.privateRequest(t, a, "GET", "/notifications?before="+page.Next, "", nil))
	if len(last.Items) != 1 || last.Next != "" {
		t.Fatal(last)
	}
	if r := f.privateRequest(t, a, "GET", "/notifications?before="+foreign.ID, "", nil); r.status != 404 {
		t.Fatal("foreign cursor", r.status)
	}
	id := out[0].ID
	if r := f.privateRequest(t, b, "PATCH", "/notifications/"+id+"/read", `"1"`, map[string]bool{"read": true}); r.status != 404 {
		t.Fatal("foreign read", r.status)
	}
	seen := readNotification(t, f.privateRequest(t, a, "PATCH", "/notifications/"+id+"/read", `"1"`, map[string]bool{"read": true}, "notification-read-once-key"))
	if seen.ReadAt == nil || seen.Version != 2 {
		t.Fatal(seen)
	}
	again := readNotification(t, f.privateRequest(t, a, "PATCH", "/notifications/"+id+"/read", `"1"`, map[string]bool{"read": true}, "notification-read-once-key"))
	if again.Version != 2 {
		t.Fatal("read replay")
	}
	if r := f.privateRequest(t, a, "PATCH", "/notifications/"+id+"/read", `"1"`, map[string]bool{"read": false}); r.status != 409 {
		t.Fatal("stale read", r.status)
	}
	cutoff := f.clock.Now()
	f.clock.Advance(time.Second)
	newID := testkit.NewUser().ID
	emit(who.ID, notifications.Event{Kind: "achievement", SourceKind: "achievement", SourceID: newID, SourceRevision: 1, EventKey: "achievement/" + newID, URL: "/app/achievements"})
	r := f.privateRequest(t, a, "POST", "/notifications/read-all", "", map[string]time.Time{"seen_before": cutoff})
	if r.status != 200 {
		t.Fatal(r.status, string(r.body))
	}
	if readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil)).Unread != 1 {
		t.Fatal("unseen event marked read")
	}
	if readNotifications(t, f.privateRequest(t, b, "GET", "/notifications", "", nil)).Unread != 1 {
		t.Fatal("other owner modified")
	}
	if _, e := f.pool.Exec(f.ctx, `UPDATE notifications SET source_id='changed' WHERE owner_id=$1 AND id=$2`, who.ID, id); e == nil {
		t.Fatal("source history rewritten")
	}
}
