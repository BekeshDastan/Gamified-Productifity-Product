//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/seasons"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"sync"
	"testing"
	"time"
)

func seasonInput() seasons.CreateInput {
	return seasons.CreateInput{OperationID: testkit.NewUser().ID, Input: seasons.Input{Title: "Осенний ритм", Theme: "Учёба и практика", DesiredResult: "Завершить выбранный небольшой проект", StartsOn: "2026-10-12", EndsOn: "2026-11-10", Primary: true}}
}
func readSeason(t *testing.T, r authResponse) seasons.Season {
	t.Helper()
	var out struct{ Data seasons.Season }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestSeasonPrimaryActivationRaceAndPermanentCreate(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "seasons-primary@example.test")
	b, _ := f.signedIn(t, "seasons-other@example.test")
	inputs := []seasons.CreateInput{seasonInput(), seasonInput()}
	runs := []seasons.Season{}
	for _, in := range inputs {
		runs = append(runs, readSeason(t, f.privateRequest(t, a, "POST", "/seasons", "", in)))
	}
	results := make([]authResponse, 2)
	var wg sync.WaitGroup
	for i := range runs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = f.privateRequest(t, a, "POST", "/seasons/"+runs[i].ID+"/activate", `"1"`, seasons.Confirmation{Confirm: true})
		}(i)
	}
	wg.Wait()
	won := -1
	for i, r := range results {
		if r.status == 200 {
			if won >= 0 {
				t.Fatal("two primary seasons")
			}
			won = i
		} else if r.status != 409 {
			t.Fatal(r.status, string(r.body))
		}
	}
	if won < 0 {
		t.Fatal("no season activated")
	}
	active := readSeason(t, results[won])
	if active.State != "active" || active.ActivatedAt == nil || active.Timezone != "UTC" {
		t.Fatal(active)
	}
	repeat := readSeason(t, f.privateRequest(t, a, "POST", "/seasons/"+active.ID+"/activate", `"1"`, seasons.Confirmation{Confirm: true}))
	if repeat.Version != active.Version {
		t.Fatal("duplicate activation")
	}
	if r := f.privateRequest(t, b, "GET", "/seasons/"+active.ID, "", nil); r.status != 404 {
		t.Fatal("foreign season", r.status)
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM personal_seasons WHERE owner_id=$1 AND state='active' AND is_primary", who.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	changed := inputs[won]
	changed.Title = "Другой сезон"
	if r := f.privateRequest(t, a, "POST", "/seasons", "", changed); r.status != 409 {
		t.Fatal("operation reused", r.status)
	}
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "seasons-primary@example.test")
	again := readSeason(t, f.privateRequest(t, a, "POST", "/seasons", "", inputs[won]))
	if again.ID != active.ID || again.Version != active.Version {
		t.Fatal("permanent replay")
	}
	if _, e := f.pool.Exec(f.ctx, "UPDATE personal_seasons SET ends_on=ends_on+1 WHERE owner_id=$1 AND id=$2", who.ID, active.ID); e == nil {
		t.Fatal("season definition rewritten")
	}
}
func TestSeasonDateLimitsFutureActivationAndSecondary(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "seasons-dates@example.test")
	for _, edit := range []func(*seasons.CreateInput){func(v *seasons.CreateInput) { v.EndsOn = "2027-10-12" }, func(v *seasons.CreateInput) { v.EndsOn = "2026-10-11" }, func(v *seasons.CreateInput) { v.StartsOn = "2026-10-11" }, func(v *seasons.CreateInput) { v.Title = "\x00" }, func(v *seasons.CreateInput) { v.OperationID = "bad" }} {
		in := seasonInput()
		edit(&in)
		if r := f.privateRequest(t, a, "POST", "/seasons", "", in); r.status != 422 {
			t.Fatal(r.status, string(r.body))
		}
	}
	in := seasonInput()
	in.StartsOn = "2026-10-13"
	future := readSeason(t, f.privateRequest(t, a, "POST", "/seasons", "", in))
	if r := f.privateRequest(t, a, "POST", "/seasons/"+future.ID+"/activate", `"1"`, seasons.Confirmation{Confirm: true}); r.status != 409 {
		t.Fatal("future activation", r.status)
	}
	for i := 0; i < 2; i++ {
		in := seasonInput()
		if i == 1 {
			in.Primary = false
		}
		v := readSeason(t, f.privateRequest(t, a, "POST", "/seasons", "", in))
		if r := f.privateRequest(t, a, "POST", "/seasons/"+v.ID+"/activate", `"1"`, seasons.Confirmation{}); r.status != 422 {
			t.Fatal("activation without consent", r.status)
		}
		readSeason(t, f.privateRequest(t, a, "POST", "/seasons/"+v.ID+"/activate", `"1"`, seasons.Confirmation{Confirm: true}))
	}
}
