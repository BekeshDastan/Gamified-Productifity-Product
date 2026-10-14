//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/focus"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/profile"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"sync"
	"testing"
	"time"
)

func readBalance(t *testing.T, r authResponse) analytics.BalanceView {
	t.Helper()
	var v struct{ Data analytics.BalanceView }
	if r.status != 200 || json.Unmarshal(r.body, &v) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return v.Data
}
func TestAnalyticsPersonalTargetsKeepUnitsUnknownAndRange(t *testing.T) {
	limits := auth.DefaultLimits()
	limits.UserRequests = 600
	f := newAuthFixture(t, limits)
	a, who := f.signedIn(t, "personal-targets@example.test")
	b, _ := f.signedIn(t, "personal-targets-other@example.test")
	areas := []profile.Label{}
	for _, name := range []string{"Практика задач", "Практика фокуса", "Чтение"} {
		r := f.privateRequest(t, a, "POST", "/areas", "", profile.LabelInput{Name: name, Color: "#6478c8", Icon: "book"})
		var v struct{ Data profile.Label }
		if r.status != 200 || json.Unmarshal(r.body, &v) != nil {
			t.Fatal(r.status, string(r.body))
		}
		areas = append(areas, v.Data)
	}
	input := questInput("Одна задача")
	input.AreaID = &areas[0].ID
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	start := focusInput()
	start.AreaID = &areas[1].ID
	s := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", start))
	f.clock.Advance(120 * time.Second)
	s = focusAct(t, f, a, s, "finish")
	readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), focus.ConfirmInput{Result: "progress"}))
	hin := habitInput()
	hin.AreaID = &areas[2].ID
	h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", hin))
	readHabit(t, f.privateRequest(t, a, "POST", "/habits/"+h.ID+"/logs", habitVersion(h), habits.LogInput{PeriodID: h.Periods[0].ID, Amount: 20, OccurredAt: f.clock.Now()}))
	scope := analytics.Range{From: "2026-10-12", To: "2026-10-13"}
	path := "/analytics/balance?from=" + scope.From + "&to=" + scope.To
	empty := readBalance(t, f.privateRequest(t, a, "GET", path, "", nil))
	for _, axis := range empty.Axes {
		if axis.Target != nil || axis.Percent != nil || axis.Version != 0 {
			t.Fatal("invented target", axis)
		}
	}
	for i, metric := range []string{"tasks", "focus_minutes", "habit_norms"} {
		in := analytics.BalanceTargetInput{Range: scope, Metric: metric, Target: 2, Confirm: true}
		r := f.privateRequest(t, a, "PUT", "/analytics/targets/"+areas[i].ID, `"0"`, in)
		if r.status != 200 {
			t.Fatal(r.status, string(r.body))
		}
	}
	current := readBalance(t, f.privateRequest(t, a, "GET", path, "", nil))
	axes := map[string]analytics.BalanceAxis{}
	for _, v := range current.Axes {
		axes[v.ID] = v
	}
	if *axes[areas[0].ID].Percent != 50 || *axes[areas[1].ID].Percent != 100 || *axes[areas[2].ID].Percent != 50 || *axes[areas[1].ID].Actual != 2 || axes[areas[1].ID].Unit != "minutes" || axes[areas[2].ID].Unit != "periods" {
		t.Fatal("mixed units", axes)
	}
	for _, area := range areas {
		if len(axes[area.ID].Sources) != 1 {
			t.Fatal("no exact evidence", axes[area.ID])
		}
	}
	zero := analytics.BalanceTargetInput{Range: scope, Metric: "tasks", Target: 0, Confirm: true}
	if r := f.privateRequest(t, a, "PUT", "/analytics/targets/"+areas[0].ID, `"1"`, zero); r.status != 200 {
		t.Fatal(r.status)
	}
	for _, axis := range readBalance(t, f.privateRequest(t, a, "GET", path, "", nil)).Axes {
		if axis.ID == areas[0].ID && axis.Percent != nil {
			t.Fatal("zero denominator", axis)
		}
	}
	if r := f.privateRequest(t, a, "PUT", "/analytics/targets/"+areas[0].ID, `"1"`, zero); r.status != 409 {
		t.Fatal("stale target overwrote", r.status)
	}
	if r := f.privateRequest(t, b, "PUT", "/analytics/targets/"+areas[0].ID, `"0"`, zero); r.status != 404 {
		t.Fatal("foreign target", r.status)
	}
	changed := readBalance(t, f.privateRequest(t, a, "GET", "/analytics/balance?from=2026-10-12&to=2026-10-14", "", nil))
	for _, axis := range changed.Axes {
		if axis.Target != nil || axis.Percent != nil {
			t.Fatal("target silently rescaled", axis)
		}
	}
	invalid := zero
	invalid.Metric = "execute arbitrary code"
	if r := f.privateRequest(t, a, "PUT", "/analytics/targets/"+areas[0].ID, `"2"`, invalid); r.status != 422 {
		t.Fatal(r.status)
	}
	var events int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM personal_area_target_events WHERE owner_id=$1", who.ID).Scan(&events); e != nil || events != 4 {
		t.Fatal(events, e)
	}
	in := analytics.BalanceTargetInput{Range: analytics.Range{From: "2026-10-12", To: "2026-10-14"}, Metric: "tasks", Target: 5, Confirm: true}
	responses := make([]authResponse, 4)
	var wg sync.WaitGroup
	for i := range responses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			responses[i] = f.privateRequest(t, a, "PUT", "/analytics/targets/"+areas[0].ID, `"0"`, in, "personal-target-race-0001")
		}(i)
	}
	wg.Wait()
	for i, r := range responses {
		if r.status != 200 {
			t.Fatal(fmt.Sprintf("race %d: %d %s", i, r.status, r.body))
		}
	}
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM personal_area_target_events WHERE owner_id=$1", who.ID).Scan(&events); e != nil || events != 5 {
		t.Fatal("duplicate target event", events, e)
	}
}
