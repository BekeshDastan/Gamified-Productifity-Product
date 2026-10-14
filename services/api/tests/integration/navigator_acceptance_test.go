//go:build integration

package integration

import (
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"testing"
	"time"
)

func TestNavigatorTaskViewCountsServerVisibleIntervalsNotClaimedElapsed(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "view-time@example.test")
	b, _ := f.signedIn(t, "view-other@example.test")
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Шаг")))
	path := "/navigator/task-view/" + q.ID
	send := func(visible bool) {
		t.Helper()
		r := f.privateRequest(t, a, "POST", path, "", map[string]bool{"visible": visible})
		if r.status != 200 {
			t.Fatal(r.status, string(r.body))
		}
	}
	has := func(rule string) bool {
		for _, v := range readNavigator(t, f.privateRequest(t, a, "GET", "/navigator", "", nil)).Items {
			if v.Rule.ID == rule {
				return true
			}
		}
		return false
	}
	if r := f.privateRequest(t, b, "POST", path, "", map[string]bool{"visible": true}); r.status != 404 {
		t.Fatal("foreign view", r.status)
	}
	send(true)
	f.clock.Advance(time.Hour)
	f.loginAs(t, a, "view-time@example.test")
	send(true)
	if has("R07") {
		t.Fatal("hour without heartbeat invented viewing")
	}
	for i := 0; i < 121; i++ {
		f.clock.Advance(30 * time.Second)
		if i%20 == 0 {
			f.loginAs(t, a, "view-time@example.test")
		}
		send(true)
	}
	if !has("R07") {
		t.Fatal("real visible hour not recognized")
	}
	send(false)
	if has("R07") {
		t.Fatal("hidden tab treated as current viewing")
	}
	send(true)
	child := questInput("Подзадача")
	child.ParentID = &q.ID
	readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", child))
	if has("R07") {
		t.Fatal("first step ignored")
	}
}
func TestNavigatorInactiveReturnUsesActivityAndCooldown(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "inactive-return@example.test")
	f.clock.Advance(4 * 24 * time.Hour)
	f.loginAs(t, a, "inactive-return@example.test")
	page := readNavigator(t, f.privateRequest(t, a, "GET", "/navigator", "", nil))
	found := false
	for _, v := range page.Items {
		if v.Rule.ID == "R05" {
			found = true
			if v.Fact.Numbers["inactive_days"] != 4 || v.Rule.CooldownSeconds != 14*86400 {
				t.Fatal(v)
			}
		}
	}
	if !found {
		t.Fatal(page)
	}
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Возвращение")))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]any{}))
	for _, v := range readNavigator(t, f.privateRequest(t, a, "GET", "/navigator", "", nil)).Items {
		if v.Rule.ID == "R05" {
			t.Fatal("recent confirmed activity ignored")
		}
	}
}
