//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/notifications"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/jackc/pgx/v5"
	"testing"
)

func readNotificationPrefs(t *testing.T, r authResponse) notifications.Preferences {
	t.Helper()
	var out struct{ Data notifications.Preferences }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestNotificationPreferencesVersionMuteAndTimezoneScope(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "notification-prefs@example.test")
	b, _ := f.signedIn(t, "notification-prefs-other@example.test")
	p := readNotificationPrefs(t, f.privateRequest(t, a, "GET", "/notifications/preferences", "", nil))
	if p.Version != 0 || p.EffectiveTimezone != "UTC" {
		t.Fatal(p)
	}
	for _, c := range p.Channels {
		if c.Email || c.Push || !c.InApp {
			t.Fatal("external channels enabled by default")
		}
	}
	input := p.PreferencesInput
	input.Channels[0].InApp = false
	input.Channels[0].Email = true
	p = readNotificationPrefs(t, f.privateRequest(t, a, "PUT", "/notifications/preferences", `"0"`, input, "notification-prefs-once-key"))
	if p.Version != 1 || p.Channel("deadline").InApp || !p.Channel("deadline").Email {
		t.Fatal(p)
	}
	again := readNotificationPrefs(t, f.privateRequest(t, a, "PUT", "/notifications/preferences", `"0"`, input, "notification-prefs-once-key"))
	if again.Version != 1 {
		t.Fatal("repeat preferences")
	}
	if r := f.privateRequest(t, a, "PUT", "/notifications/preferences", `"0"`, input); r.status != 409 {
		t.Fatal("stale preferences", r.status)
	}
	other := readNotificationPrefs(t, f.privateRequest(t, b, "GET", "/notifications/preferences", "", nil))
	if other.Version != 0 || other.Channel("deadline").Email {
		t.Fatal("foreign preferences")
	}
	service := notifications.NewService(f.pool, f.clock)
	e := storage.WithinTx(f.ctx, f.pool, func(tx pgx.Tx) error {
		var locked string
		if e := tx.QueryRow(f.ctx, `SELECT owner_id::text FROM profiles WHERE owner_id=$1 FOR UPDATE`, who.ID).Scan(&locked); e != nil {
			return e
		}
		_, _, e := service.EmitInTx(f.ctx, tx, who.ID, notifications.Event{Kind: "deadline", SourceKind: "task", SourceID: who.ID, SourceRevision: 1, EventKey: "muted-source", URL: "/app/quests/" + who.ID})
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	page := readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil))
	if len(page.Items) != 1 || !page.Items[0].Muted || page.Items[0].ReadAt != nil || page.Unread != 0 {
		t.Fatal("mute erased history or invented reading", page)
	}
	if _, e = f.pool.Exec(f.ctx, `UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}',to_jsonb('Asia/Almaty'::text)),version=version+1 WHERE owner_id=$1`, who.ID); e != nil {
		t.Fatal(e)
	}
	p = readNotificationPrefs(t, f.privateRequest(t, a, "GET", "/notifications/preferences", "", nil))
	if p.EffectiveTimezone != "Asia/Almaty" {
		t.Fatal("profile timezone not followed")
	}
	input.UseProfileTimezone = false
	input.Timezone = "America/New_York"
	p = readNotificationPrefs(t, f.privateRequest(t, a, "PUT", "/notifications/preferences", `"1"`, input))
	if p.EffectiveTimezone != input.Timezone || p.Version != 2 {
		t.Fatal(p)
	}
	input.Channels[1].Type = input.Channels[0].Type
	if r := f.privateRequest(t, a, "PUT", "/notifications/preferences", `"2"`, input); r.status != 422 {
		t.Fatal("duplicate channel type", r.status)
	}
}
