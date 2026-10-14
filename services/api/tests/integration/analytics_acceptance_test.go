//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/focus"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"testing"
	"time"
)

func TestAnalyticsDSTAndFractionalWorkKeepCanonicalSeconds(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "analytics-dst@example.test")
	start := time.Date(2026, 10, 31, 23, 59, 59, 500000000, time.UTC)
	f.clock.Advance(start.Sub(f.clock.Now()))
	f.loginAs(t, a, "analytics-dst@example.test")
	if _, e := f.pool.Exec(f.ctx, "UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}',to_jsonb('America/New_York'::text)),version=version+1 WHERE owner_id=$1", who.ID); e != nil {
		t.Fatal(e)
	}
	s := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", focusInput()))
	f.clock.Advance(time.Second)
	s = focusAct(t, f, a, s, "pause")
	f.clock.Advance(4*time.Hour - 500*time.Millisecond)
	f.loginAs(t, a, "analytics-dst@example.test")
	s = focusAct(t, f, a, s, "resume")
	f.clock.Advance(2*time.Hour + 1500*time.Millisecond)
	f.loginAs(t, a, "analytics-dst@example.test")
	s = focusAct(t, f, a, s, "finish")
	s = readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), focus.ConfirmInput{Result: "progress"}))
	if s.ActiveSeconds != 7202 {
		t.Fatal(s)
	}
	scope := analytics.Range{From: "2026-10-31", To: "2026-11-02"}
	v := readAnalytics(t, f.privateRequest(t, a, "POST", "/analytics/rebuild", "", scope))
	if v.Timezone != "America/New_York" || v.Days[0].FocusSeconds != 1 || v.Days[1].FocusSeconds != 7201 || v.Weeks[0].FocusSeconds != 7202 {
		t.Fatal("DST or pauses inflated work", v)
	}
	if _, e := f.pool.Exec(f.ctx, "UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}',to_jsonb('UTC'::text)),version=version+1 WHERE owner_id=$1", who.ID); e != nil {
		t.Fatal(e)
	}
	v = readAnalytics(t, f.privateRequest(t, a, "GET", "/analytics?from=2026-10-31&to=2026-11-02", "", nil))
	if v.Timezone != "UTC" || v.Days[0].FocusSeconds != 0 || v.Days[1].FocusSeconds != 7202 || v.Stale {
		t.Fatal("zone rebuild lost seconds", v)
	}
}
func TestAnalyticsRollingScoreMatchesAcrossRangesAndKeepsZeroDay(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "analytics-window@example.test")
	for day := 0; day < 4; day++ {
		date := f.clock.Now().Format(time.DateOnly)
		var in planning.Input
		var q quests.Quest
		in.AvailableMinutes = 10
		if day == 2 {
			in.Rest = true
			in.AvailableMinutes = 0
		} else {
			input := questInput("Обязательство дня")
			input.EstimatedMinutes = 10
			q = readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
			in.QuestIDs = []string{q.ID}
		}
		p := readPlan(t, f.privateRequest(t, a, "PUT", "/plans/day/"+date, `"0"`, in))
		readPlan(t, f.privateRequest(t, a, "POST", "/plans/day/"+date+"/confirm", planVersion(p), struct{}{}))
		if q.ID != "" && day != 1 {
			readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
		}
		f.clock.Advance(24 * time.Hour)
		f.loginAs(t, a, "analytics-window@example.test")
	}
	all := readAnalytics(t, f.privateRequest(t, a, "POST", "/analytics/rebuild", "", analytics.Range{From: "2026-10-12", To: "2026-10-16"}))
	one := readAnalytics(t, f.privateRequest(t, a, "POST", "/analytics/rebuild", "", analytics.Range{From: "2026-10-15", To: "2026-10-16"}))
	score := one.Days[0].Scores.Discipline
	if score.Score == nil || *score.Score != 74 || score.ActiveDays != 3 || score.TaskObligations != 3 || len(score.Dates) != 3 || all.Days[1].Scores.Daily == nil || *all.Days[1].Scores.Daily != 0 || all.Days[2].Scores.Daily != nil || all.Days[2].Scores.State != "rest" {
		t.Fatal("zero/rest/window mismatch", all, one)
	}
	aJSON, _ := json.Marshal(score)
	bJSON, _ := json.Marshal(all.Days[3].Scores.Discipline)
	if string(aJSON) != string(bJSON) {
		t.Fatal("range changes discipline", score, all.Days[3].Scores.Discipline)
	}
}
func TestAnalyticsReportHistoryPaginationAndOwnerCursor(t *testing.T) {
	limits := auth.DefaultLimits()
	limits.UserRequests = 600
	f := newAuthFixture(t, limits)
	a, _ := f.signedIn(t, "report-pages@example.test")
	b, _ := f.signedIn(t, "report-pages-other@example.test")
	for i := 0; i < 26; i++ {
		p := readReportPreview(t, f.privateRequest(t, a, "POST", "/analytics/reports/preview", "", analytics.ReportInput{Kind: "week", StartsOn: "2026-10-05", Timezone: "UTC"}))
		readReport(t, f.privateRequest(t, a, "POST", "/analytics/reports", "", analytics.SaveReportInput{OperationID: testkit.NewUser().ID, PreviewID: p.ID, Confirm: true}))
		f.clock.Advance(time.Second)
	}
	page := func(path string) analytics.ReportPage {
		r := f.privateRequest(t, a, "GET", path, "", nil)
		var v struct{ Data analytics.ReportPage }
		if r.status != 200 || json.Unmarshal(r.body, &v) != nil {
			t.Fatal(r.status, string(r.body))
		}
		return v.Data
	}
	first := page("/analytics/reports")
	if len(first.Items) != 25 || first.Next == "" {
		t.Fatal(first)
	}
	next := page("/analytics/reports?before=" + first.Next)
	if len(next.Items) != 1 || next.Next != "" {
		t.Fatal(next)
	}
	if r := f.privateRequest(t, b, "GET", "/analytics/reports?before="+first.Next, "", nil); r.status != 404 {
		t.Fatal("foreign cursor", r.status)
	}
	if r := f.privateRequest(t, a, "GET", "/analytics/reports/bad", "", nil); r.status != 404 {
		t.Fatal(r.status)
	}
}
