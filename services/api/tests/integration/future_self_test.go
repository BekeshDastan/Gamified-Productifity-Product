//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/focus"
	"strings"
	"testing"
	"time"
)

func TestFutureSelfConfirmedWorkFourWeeksAndEmptyOwner(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "future@example.test")
	b, _ := f.signedIn(t, "future-other@example.test")
	read := func(r authResponse) analytics.FutureSelf {
		var out struct{ Data analytics.FutureSelf }
		if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
			t.Fatal(r.status, string(r.body))
		}
		return out.Data
	}
	v := read(f.privateRequest(t, a, "GET", "/analytics/future-self?weeks=12", "", nil))
	if v.Available || v.ProjectedHours != nil {
		t.Fatal("invented initial prediction", v)
	}
	f.clock.Advance(7 * 24 * time.Hour)
	for i := 0; i < 4; i++ {
		f.loginAs(t, a, "future@example.test")
		session := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", focusInput()))
		f.clock.Advance(4 * time.Hour)
		f.loginAs(t, a, "future@example.test")
		session = focusAct(t, f, a, session, "finish")
		readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+session.ID+"/confirm", focusVersion(session), focus.ConfirmInput{Result: "progress", Comment: "private future sentinel"}))
		f.clock.Advance(7*24*time.Hour - 4*time.Hour)
	}
	f.loginAs(t, a, "future@example.test")
	f.loginAs(t, b, "future-other@example.test")
	v = read(f.privateRequest(t, a, "GET", "/analytics/future-self?weeks=12", "", nil))
	if !v.Available || v.ObservedWeeks != 4 || *v.AverageWeeklyHours != 4 || *v.ProjectedHours != 48 {
		t.Fatal(v)
	}
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "sentinel") {
		t.Fatal("private comment leaked")
	}
	for _, w := range v.Weeks {
		if w.FocusSeconds == nil || *w.FocusSeconds != 14400 || len(w.Sources) == 0 {
			t.Fatal(w)
		}
	}
	empty := read(f.privateRequest(t, b, "GET", "/analytics/future-self?weeks=12", "", nil))
	if !empty.Available || *empty.ProjectedHours != 0 {
		t.Fatal("zero full weeks omitted", empty)
	}
	for _, w := range empty.Weeks {
		if len(w.Sources) > 0 {
			t.Fatal("foreign focus")
		}
	}
	if r := f.privateRequest(t, a, "GET", "/analytics/future-self?weeks=105", "", nil); r.status != 422 {
		t.Fatal("horizon", r.status)
	}
}
