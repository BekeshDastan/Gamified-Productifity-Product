//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/seasons"
	"testing"
	"time"
)

func TestRecoveryStatusKeepsMissAndPartialMinimumSeparate(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "honest-recovery@example.test")
	b, _ := f.signedIn(t, "other-honest@example.test")
	h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", habitInput()))
	path := "/habits/" + h.ID
	log := func(amount float64) {
		p := h.Periods[len(h.Periods)-1]
		h = readHabit(t, f.privateRequest(t, a, "POST", path+"/logs", habitVersion(h), habits.LogInput{PeriodID: p.ID, Amount: amount, OccurredAt: f.clock.Now()}))
	}
	status := func() seasons.RecoveryStatus {
		r := f.privateRequest(t, a, "GET", "/recovery/status", "", nil)
		var v struct{ Data seasons.RecoveryStatus }
		if r.status != 200 || json.Unmarshal(r.body, &v) != nil {
			t.Fatal(r.status, string(r.body))
		}
		return v.Data
	}
	log(20)
	f.clock.Advance(48 * time.Hour)
	f.loginAs(t, a, "honest-recovery@example.test")
	h = readHabit(t, f.privateRequest(t, a, "GET", path, "", nil))
	log(1)
	eligibility := readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil))
	if eligibility.Eligible || eligibility.LastResultAt == nil || !eligibility.LastResultAt.Equal(f.clock.Now()) {
		t.Fatal("small step ignored by recovery", eligibility)
	}
	v := status()
	if !v.Complete || len(v.Habits) != 1 {
		t.Fatal(v)
	}
	s := v.Habits[0]
	if s.Stats.Current != 0 || s.Stats.Best != 1 || s.Stats.Missed != 1 || s.MinimumMet == nil || !*s.MinimumMet || *s.CurrentAmount != 1 || *s.FullTarget != 20 {
		t.Fatal("partial minimum repairs streak", s)
	}
	log(19)
	s = status().Habits[0]
	if s.Stats.Current != 1 || s.Stats.Best != 1 || s.Stats.Missed != 1 || !s.Stats.Recovering || s.Periods[1].State != "missed" {
		t.Fatal("lost real miss", s)
	}
	f.clock.Advance(72 * time.Hour)
	f.loginAs(t, a, "honest-recovery@example.test")
	eligible := readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil))
	if eligible.Offer == nil {
		t.Fatal(eligible)
	}
	readRecoveryOffer(t, f.privateRequest(t, a, "POST", "/recovery/comeback/"+eligible.Offer.ID+"/decide", `"1"`, seasons.RecoveryDecision{Timezone: eligible.Timezone, Accept: true, Steps: eligible.Offer.Steps}))
	v = status()
	if len(v.Comebacks) != 1 || len(v.Habits[0].Periods) < 5 || v.Habits[0].Stats.Current != 0 || v.Habits[0].Stats.Best != 1 {
		t.Fatal("comeback rewrote missed periods", v)
	}
	before := v.Comebacks[0].Member.Progress
	v = status()
	if string(before) != string(v.Comebacks[0].Member.Progress) {
		t.Fatal("read changes progress")
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM habit_logs WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 3 {
		t.Fatal("fabricated log", n, e)
	}
	f.loginAs(t, b, "other-honest@example.test")
	r := f.privateRequest(t, b, "GET", "/recovery/status", "", nil)
	var other struct{ Data seasons.RecoveryStatus }
	if r.status != 200 || json.Unmarshal(r.body, &other) != nil || len(other.Data.Habits) != 0 || len(other.Data.Comebacks) != 0 {
		t.Fatal("foreign evidence", r.status, string(r.body))
	}
}
