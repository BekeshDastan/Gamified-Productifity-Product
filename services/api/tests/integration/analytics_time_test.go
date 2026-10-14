//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/focus"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"testing"
	"time"
)

func TestAnalyticsActualUnknownManualOverrideAndPauseExclusion(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "analytics-time@example.test")
	in := questInput("С измерением")
	in.EstimatedMinutes = 10
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", in))
	start := focusInput()
	start.QuestID = &q.ID
	s := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", start))
	f.clock.Advance(90 * time.Second)
	s = focusAct(t, f, a, s, "pause")
	f.clock.Advance(time.Hour)
	f.loginAs(t, a, "analytics-time@example.test")
	s = focusAct(t, f, a, s, "resume")
	f.clock.Advance(30 * time.Second)
	s = focusAct(t, f, a, s, "finish")
	readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), focus.ConfirmInput{Result: "progress"}))
	edit := q.Input
	edit.ActualTimeMode = "manual_override"
	edit.ActualMinutes = 3
	q = readQuest(t, f.privateRequest(t, a, "PATCH", "/quests/"+q.ID, version(q), edit))
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	missing := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Без замера")))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+missing.ID+"/complete", version(missing), quests.Transition{}))
	zero := questInput("Явно ноль минут")
	zero.ActualTimeMode = "manual_override"
	zero.ActualMinutes = 0
	z := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", zero))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+z.ID+"/complete", version(z), quests.Transition{}))
	v := readAnalytics(t, f.privateRequest(t, a, "POST", "/analytics/rebuild", "", analytics.Range{From: "2026-10-12", To: "2026-10-13"}))
	d := v.Days[0]
	if d.FocusSeconds != 120 || d.ComparableTasks != 1 || d.ComparableActualSeconds != 180 || d.ComparableEstimatedSeconds != 600 || d.ActualKnownTasks != 2 || d.EstimatedTasks != 1 {
		t.Fatal("manual and focus double-counted or unknown coerced", d)
	}
	records := map[string]analytics.TaskTime{}
	for _, r := range d.TaskTimes {
		records[r.ID] = r
	}
	measured := records[q.ID]
	if measured.ActualSeconds == nil || *measured.ActualSeconds != 180 || measured.FocusSeconds != 120 || measured.ConfirmedSessions != 1 || measured.ManualMinutes == nil || *measured.ManualMinutes != 3 {
		t.Fatal(measured)
	}
	if records[missing.ID].ActualSeconds != nil || records[missing.ID].EstimatedMinutes != nil {
		t.Fatal("missing is zero", records[missing.ID])
	}
	if records[z.ID].ActualSeconds == nil || *records[z.ID].ActualSeconds != 0 {
		t.Fatal("explicit zero lost", records[z.ID])
	}
	edit = q.Input
	edit.EstimatedMinutes = 100
	readQuest(t, f.privateRequest(t, a, "PATCH", "/quests/"+q.ID, version(q), edit))
	v = readAnalytics(t, f.privateRequest(t, a, "POST", "/analytics/rebuild", "", analytics.Range{From: "2026-10-12", To: "2026-10-13"}))
	for _, r := range v.Days[0].TaskTimes {
		if r.ID == q.ID && *r.EstimatedMinutes != 10 {
			t.Fatal("completion estimate rewritten", r)
		}
	}
}
func TestAnalyticsTemplateSamplesRequireRecordedOriginAndFiveKnownPairs(t *testing.T) {
	limits := auth.DefaultLimits()
	limits.UserRequests = 600
	f := newAuthFixture(t, limits)
	a, _ := f.signedIn(t, "analytics-template@example.test")
	other, _ := f.signedIn(t, "analytics-template-private@example.test")
	input := questInput("Одинаковое название")
	input.EstimatedMinutes = 10
	seed := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	r := f.privateRequest(t, a, "POST", "/templates/quests", "", quests.TemplateSource{Name: "Моя практика", QuestID: seed.ID})
	var template struct{ Data quests.Template }
	if r.status != 200 || json.Unmarshal(r.body, &template) != nil {
		t.Fatal(r.status, string(r.body))
	}
	readSamples := func() analytics.TemplateSamples {
		r := f.privateRequest(t, a, "GET", "/analytics/templates?from=2026-10-12&to=2026-10-13", "", nil)
		var v struct{ Data analytics.TemplateSamples }
		if r.status != 200 || json.Unmarshal(r.body, &v) != nil {
			t.Fatal(r.status, string(r.body))
		}
		return v.Data
	}
	for i := 0; i < 5; i++ {
		q := readQuest(t, f.privateRequest(t, a, "POST", "/templates/quests/"+template.Data.ID+"/apply", `"1"`, quests.TemplateDates{}))
		edit := q.Input
		edit.ActualTimeMode = "manual_override"
		edit.ActualMinutes = 20
		q = readQuest(t, f.privateRequest(t, a, "PATCH", "/quests/"+q.ID, version(q), edit))
		readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
		if i == 3 {
			sample := readSamples().Items[0]
			if sample.Count != 4 || sample.Sufficient {
				t.Fatal("sample threshold", sample)
			}
		}
	}
	// Same title without a template-origin row is never assigned to a sample by guesswork.
	edit := seed.Input
	edit.ActualTimeMode = "manual_override"
	edit.ActualMinutes = 100
	seed = readQuest(t, f.privateRequest(t, a, "PATCH", "/quests/"+seed.ID, version(seed), edit))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+seed.ID+"/complete", version(seed), quests.Transition{}))
	sample := readSamples().Items[0]
	if sample.Count != 5 || !sample.Sufficient || sample.Factor == nil || *sample.Factor != 2 || *sample.AverageActualSeconds != 1200 || *sample.AverageEstimatedMinutes != 10 || len(sample.QuestIDs) != 5 {
		t.Fatal(sample)
	}
	if r := f.privateRequest(t, a, "DELETE", "/templates/quests/"+template.Data.ID, `"1"`, struct{}{}); r.status != 200 {
		t.Fatal(r.status, string(r.body))
	}
	after := readSamples().Items[0]
	if after.Name != "Моя практика" || after.Version != 1 || after.Count != 5 {
		t.Fatal("deleted template lost original evidence", after)
	}
	r = f.privateRequest(t, other, "GET", "/analytics/templates?from=2026-10-12&to=2026-10-13", "", nil)
	var list struct{ Data analytics.TemplateSamples }
	if r.status != 200 || json.Unmarshal(r.body, &list) != nil || len(list.Data.Items) != 0 {
		t.Fatal("template sample leak", r.status, string(r.body))
	}
}
