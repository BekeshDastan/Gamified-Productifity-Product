//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/seasons"
	"testing"
	"time"
)

func TestSeasonPaginationClosedReadAndPrivateCursor(t *testing.T) {
	limits := auth.DefaultLimits()
	limits.UserRequests = 600
	f := newAuthFixture(t, limits)
	a, _ := f.signedIn(t, "season-pages@example.test")
	b, _ := f.signedIn(t, "season-pages-other@example.test")
	var last seasons.Season
	for i := 0; i < 26; i++ {
		in := seasonInput()
		in.Title = fmt.Sprintf("Сезон %d", i)
		last = readSeason(t, f.privateRequest(t, a, "POST", "/seasons", "", in))
		f.clock.Advance(time.Second)
	}
	page := func(path string) seasons.Page {
		r := f.privateRequest(t, a, "GET", path, "", nil)
		var v struct{ Data seasons.Page }
		if r.status != 200 || json.Unmarshal(r.body, &v) != nil {
			t.Fatal(r.status, string(r.body))
		}
		return v.Data
	}
	first := page("/seasons")
	if len(first.Items) != 25 || first.Next == "" || first.Items[0].ID != last.ID {
		t.Fatal(first)
	}
	next := page("/seasons?before=" + first.Next)
	if len(next.Items) != 1 || next.Next != "" {
		t.Fatal(next)
	}
	for _, path := range []string{"/seasons?before=" + first.Next, "/seasons/" + last.ID, "/seasons/" + last.ID + "/overview", "/seasons/" + last.ID + "/summary"} {
		if r := f.privateRequest(t, b, "GET", path, "", nil); r.status != 404 {
			t.Fatal("foreign season or cursor", path, r.status)
		}
	}
	if r := f.privateRequest(t, a, "GET", "/seasons?before=bad", "", nil); r.status != 404 {
		t.Fatal(r.status)
	}
	if r := f.privateRequest(t, a, "GET", "/seasons/"+last.ID+"/summary", "", nil); r.status != 404 {
		t.Fatal("open report invented", r.status)
	}
	if r := f.privateRequest(t, a, "POST", "/seasons/"+last.ID+"/archive", `"1"`, seasons.ArchiveInput{Confirm: true, Archived: true}); r.status != 409 {
		t.Fatal("archived live season", r.status)
	}
	if r := f.privateRequest(t, a, "POST", "/seasons/"+last.ID+"/close", `"1"`, seasons.CloseInput{Confirm: false, Reason: "Не подтверждаю"}); r.status != 422 {
		t.Fatal("unconfirmed close", r.status)
	}
	closed := readSeasonSummary(t, f.privateRequest(t, a, "POST", "/seasons/"+last.ID+"/close", `"1"`, seasons.CloseInput{Confirm: true, Reason: "Не начинаю этот сезон"}))
	if closed.Overview.Season.State != "cancelled" || closed.Activity.From != nil {
		t.Fatal(closed)
	}
	if r := f.privateRequest(t, a, "POST", "/seasons/"+last.ID+"/activate", `"2"`, seasons.Confirmation{Confirm: true}); r.status != 409 {
		t.Fatal("reopened closed season", r.status)
	}
}
func TestRecoveryInvalidConsentAndExpiredOfferCannotCreateObligations(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "recovery-expired@example.test")
	f.clock.Advance(72 * time.Hour)
	f.loginAs(t, a, "recovery-expired@example.test")
	offer := readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil))
	path := "/recovery/comeback/" + offer.Offer.ID + "/decide"
	in := seasons.RecoveryDecision{Accept: true, Timezone: offer.Timezone, Steps: offer.Offer.Steps}
	for _, amount := range []int{0, 16} {
		bad := in
		bad.Steps = append([]seasons.RecoveryStep(nil), in.Steps...)
		bad.Steps[0].EstimatedMinutes = amount
		if r := f.privateRequest(t, a, "POST", path, `"1"`, bad); r.status != 422 {
			t.Fatal("invalid step duration", r.status)
		}
	}
	bad := in
	bad.Steps = nil
	if r := f.privateRequest(t, a, "POST", path, `"1"`, bad); r.status != 422 {
		t.Fatal("missing steps", r.status)
	}
	bad = in
	bad.Steps = append([]seasons.RecoveryStep(nil), in.Steps...)
	bad.Steps[0].Title = "bad\x00title"
	if r := f.privateRequest(t, a, "POST", path, `"1"`, bad); r.status != 422 {
		t.Fatal("NUL request", r.status)
	}
	if r := f.privateRequest(t, a, "POST", path, `"2"`, in); r.status != 409 {
		t.Fatal("stale offer version", r.status)
	}
	f.clock.Advance(14 * 24 * time.Hour)
	f.loginAs(t, a, "recovery-expired@example.test")
	if r := f.privateRequest(t, a, "POST", path, `"1"`, in); r.status != 409 {
		t.Fatal("expired offer accepted", r.status)
	}
	next := readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil))
	if next.Offer.ID == offer.Offer.ID {
		t.Fatal("expired proposal reused")
	}
	history := f.privateRequest(t, a, "GET", "/recovery/comeback/history", "", nil)
	var list struct {
		Data struct{ Items []seasons.RecoveryOffer }
	}
	if history.status != 200 || json.Unmarshal(history.body, &list) != nil || len(list.Data.Items) != 2 {
		t.Fatal(history.status, string(history.body))
	}
	if list.Data.Items[1].State != "expired" {
		t.Fatal(list)
	}
	var n int
	for _, table := range []string{"goals", "quests", "challenge_runs", "reward_ledger"} {
		if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table+" WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 0 {
			t.Fatal("unaccepted obligation", table, n, e)
		}
	}
}
