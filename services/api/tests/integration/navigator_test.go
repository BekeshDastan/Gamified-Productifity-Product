//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/navigator"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"strings"
	"testing"
	"time"
)

func readNavigator(t *testing.T, r authResponse) navigator.Page {
	t.Helper()
	var out struct{ Data navigator.Page }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestNavigatorEvidenceCooldownSnoozeOptOutAndPrivacy(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "navigator@example.test")
	b, _ := f.signedIn(t, "navigator-other@example.test")
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Private task name sentinel")))
	for _, date := range []string{"2026-10-13", "2026-10-14", "2026-10-15"} {
		d := date
		q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/postpone", version(q), quests.Transition{Reason: "private reason sentinel", PlannedDate: &d}))
	}
	page := readNavigator(t, f.privateRequest(t, a, "GET", "/navigator", "", nil))
	if len(page.Rules) != 15 || len(page.Items) > 3 {
		t.Fatal(page)
	}
	var c navigator.Card
	for _, v := range page.Items {
		if v.Rule.ID == "R01" {
			c = v
		}
	}
	if c.ID == "" || c.Fact.Numbers["postponements"] != 3 || c.Fact.Sources[0].ID != q.ID {
		t.Fatal(page)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), "sentinel") {
		t.Fatal("private text leaked")
	}
	again := readNavigator(t, f.privateRequest(t, a, "GET", "/navigator", "", nil))
	if again.Items[0].ID != page.Items[0].ID {
		t.Fatal("cooldown duplicate")
	}
	decision := map[string]any{"action": "snooze", "hours": 24}
	if r := f.privateRequest(t, b, "POST", "/navigator/cards/"+c.ID+"/decision", `"1"`, decision); r.status != 404 {
		t.Fatal("foreign card", r.status)
	}
	if r := f.privateRequest(t, a, "POST", "/navigator/cards/"+c.ID+"/decision", `"1"`, decision); r.status != 200 {
		t.Fatal(r.status, string(r.body))
	}
	for _, v := range readNavigator(t, f.privateRequest(t, a, "GET", "/navigator", "", nil)).Items {
		if v.Rule.ID == "R01" {
			t.Fatal("snooze ignored")
		}
	}
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "navigator@example.test")
	var returned navigator.Card
	for _, v := range readNavigator(t, f.privateRequest(t, a, "GET", "/navigator", "", nil)).Items {
		if v.Rule.ID == "R01" {
			returned = v
		}
	}
	if returned.ID != c.ID {
		t.Fatal("snooze lost cooldown")
	}
	setting := navigator.Setting{RuleID: "R01", Disabled: true}
	if r := f.privateRequest(t, a, "PUT", "/navigator/rules/R01", `"0"`, setting); r.status != 200 {
		t.Fatal(r.status, string(r.body))
	}
	f.clock.Advance(8 * 24 * time.Hour)
	f.loginAs(t, a, "navigator@example.test")
	for _, v := range readNavigator(t, f.privateRequest(t, a, "GET", "/navigator", "", nil)).Items {
		if v.Rule.ID == "R01" {
			t.Fatal("opt-out ignored")
		}
	}
	var cards int
	if e := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM navigator_cards WHERE owner_id=$1 AND rule_id='R01'`, who.ID).Scan(&cards); e != nil || cards != 1 {
		t.Fatal(cards, e)
	}
}
