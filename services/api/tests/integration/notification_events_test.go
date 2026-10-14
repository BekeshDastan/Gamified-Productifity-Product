//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/focus"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/notifications"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
	"time"
)

func TestNotificationEventsAchievementCorrectionFocusAndRecipientPrivacy(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "events-a@example.test")
	b, other := f.signedIn(t, "events-b@example.test")
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("PRIVATE_TASK_SENTINEL")))
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	if r := f.privateRequest(t, a, "GET", "/achievements", "", nil); r.status != 200 {
		t.Fatal(r.status, string(r.body))
	}
	page := readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil))
	if len(page.Items) == 0 {
		t.Fatal("achievement hook missing")
	}
	for _, n := range page.Items {
		if n.Kind != "achievement" {
			t.Fatal(n)
		}
	}
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/reopen", version(q), quests.Transition{Reason: "PRIVATE_DIARY_REASON"}))
	if r := f.privateRequest(t, a, "GET", "/achievements", "", nil); r.status != 200 {
		t.Fatal(r.status, string(r.body))
	}
	page = readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil))
	for _, n := range page.Items {
		if n.Active {
			t.Fatal("revoked source remains active")
		}
	}
	in := focusInput()
	in.Title = "PRIVATE_FOCUS_SENTINEL"
	session := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", in))
	f.clock.Advance(time.Minute)
	session = focusAct(t, f, a, session, "finish")
	page = readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil))
	found := false
	for _, n := range page.Items {
		if n.Kind == "focus_result" && n.Active && n.SourceID == session.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("focus hook missing")
	}
	confirm := focus.ConfirmInput{Result: "rest"}
	readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+session.ID+"/confirm", focusVersion(session), confirm))
	page = readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil))
	for _, n := range page.Items {
		if n.Kind == "focus_result" && n.Active {
			t.Fatal("confirmed focus still active")
		}
	}
	s := notifications.NewService(f.pool, f.clock)
	id := testkit.NewUser().ID
	tx, e := f.pool.Begin(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(f.ctx)
	if _, e = tx.Exec(f.ctx, `SELECT owner_id FROM profiles WHERE owner_id=$1 FOR UPDATE`, other.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.InviteCreated(f.ctx, tx, other.ID, id, 1); e != nil {
		t.Fatal(e)
	}
	var count int
	_ = tx.QueryRow(f.ctx, `SELECT count(*) FROM notifications WHERE owner_id=$1`, other.ID).Scan(&count)
	if count != 0 {
		t.Fatal("forged invite source")
	}
	if _, e = tx.Exec(f.ctx, `INSERT INTO notification_invite_sources(recipient_id,id,revision,expires_at,created_at) VALUES($1,$2,1,$3,$4)`, other.ID, id, f.clock.Now().Add(time.Hour), f.clock.Now()); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e = s.InviteCreated(f.ctx, tx, other.ID, id, 1); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(f.ctx); e != nil {
		t.Fatal(e)
	}
	partner := readNotifications(t, f.privateRequest(t, b, "GET", "/notifications", "", nil))
	if len(partner.Items) != 1 || partner.Items[0].Kind != "invite" {
		t.Fatal(partner)
	}
	raw, _ := json.Marshal(partner)
	if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(string(raw), who.Email) {
		t.Fatal("partner private data leaked")
	}
	if r := f.privateRequest(t, a, "GET", "/notifications/"+partner.Items[0].ID, "", nil); r.status != 404 {
		t.Fatal("foreign notification")
	}
	if r := f.privateRequest(t, a, "GET", "/notifications/"+partner.Items[0].ID+"/source", "", nil); r.status != 404 {
		t.Fatal("foreign source")
	}
	for _, n := range page.Items {
		if n.Kind == "focus_result" {
			r := f.privateRequest(t, a, "GET", "/notifications/"+n.ID+"/source", "", nil)
			if r.status != 200 || !strings.Contains(string(r.body), `"available":false`) {
				t.Fatal("stale source", r.status, string(r.body))
			}
		}
	}
	_ = pgx.ErrNoRows
}
