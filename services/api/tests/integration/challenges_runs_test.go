//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"sync"
	"testing"
	"time"
)

func readChallenge(t *testing.T, r authResponse) challenges.Run {
	t.Helper()
	var out struct{ Data challenges.Run }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func challengeInput(t *testing.T, index int) challenges.CreateInput {
	t.Helper()
	catalog, e := challenges.ReadCatalog()
	if e != nil {
		t.Fatal(e)
	}
	template := catalog.Challenges[index]
	return challenges.CreateInput{OperationID: testkit.NewUser().ID, TemplateID: &template.ID, Input: challenges.RuleInput{Config: template.Config, StartsOn: "2026-10-12", EndsOn: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC).AddDate(0, 0, template.Days-1).Format("2006-01-02")}}
}
func TestChallengeRunsFreezeRulesAndPermanentStartReplay(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "challenge-run@example.test")
	b, _ := f.signedIn(t, "challenge-run-other@example.test")
	input := challengeInput(t, 0)
	results := make([]authResponse, 5)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = f.privateRequest(t, a, "POST", "/challenges/runs", "", input)
		}(i)
	}
	wg.Wait()
	first := readChallenge(t, results[0])
	for _, r := range results {
		v := readChallenge(t, r)
		if v.ID != first.ID || v.RulesHash != first.RulesHash {
			t.Fatal("duplicate run")
		}
	}
	if first.Rules.Name == "" || first.Rules.Visibility != "private" || len(first.RulesHash) != 64 || first.CatalogVersion != challenges.CatalogVersion {
		t.Fatal(first)
	}
	if _, e := f.pool.Exec(f.ctx, `UPDATE challenge_runs SET rules=jsonb_set(rules,'{target}','999') WHERE owner_id=$1`, who.ID); e == nil {
		t.Fatal("mutable rules")
	}
	if r := f.privateRequest(t, a, "PUT", "/challenges/runs/"+first.ID+"/rules", `"1"`, input.Input); r.status != 409 {
		t.Fatal("editable rules", r.status)
	}
	if r := f.privateRequest(t, b, "GET", "/challenges/runs/"+first.ID, "", nil); r.status != 404 {
		t.Fatal("foreign read", r.status)
	}
	if _, e := f.pool.Exec(f.ctx, `UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}','"Asia/Almaty"') WHERE owner_id=$1`, who.ID); e != nil {
		t.Fatal(e)
	}
	restored := readChallenge(t, f.privateRequest(t, a, "GET", "/challenges/runs/"+first.ID, "", nil))
	if restored.Rules.Timezone != first.Rules.Timezone || restored.EndsAt != first.EndsAt {
		t.Fatal("settings changed rules")
	}
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "challenge-run@example.test")
	replay := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", input))
	if replay.ID != first.ID {
		t.Fatal("expired replay")
	}
	input.Input.Config.Target++
	if r := f.privateRequest(t, a, "POST", "/challenges/runs", "", input); r.status != 409 {
		t.Fatal("different request", r.status)
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM challenge_runs WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
}
func TestChallengeRunSourcesBoundsAndOwnerCursor(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "challenge-source@example.test")
	b, _ := f.signedIn(t, "challenge-source-other@example.test")
	foreign := readLabel(t, f.privateRequest(t, b, "POST", "/areas", "", map[string]any{"name": "Чужое", "color": "#6478c8", "icon": "book"}))
	input := challengeInput(t, 0)
	input.Input.AreaID = &foreign.ID
	if r := f.privateRequest(t, a, "POST", "/challenges/runs", "", input); r.status != 404 {
		t.Fatal("foreign area", r.status, string(r.body))
	}
	own := readLabel(t, f.privateRequest(t, a, "POST", "/areas", "", map[string]any{"name": "Мой фокус", "color": "#6478c8", "icon": "book"}))
	input.Input.AreaID = &own.ID
	run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", input))
	if run.Rules.AreaID == nil || *run.Rules.AreaID != own.ID {
		t.Fatal(run)
	}
	invalid := challengeInput(t, 0)
	invalid.Input.Unit = "km"
	if r := f.privateRequest(t, a, "POST", "/challenges/runs", "", invalid); r.status != 422 {
		t.Fatal("unit", r.status)
	}
	invalid = challengeInput(t, 4)
	if r := f.privateRequest(t, a, "POST", "/challenges/runs", "", invalid); r.status != 422 {
		t.Fatal("habit missing", r.status)
	}
	habit := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", habitInput()))
	invalid.Input.HabitID = &habit.ID
	readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", invalid))
	var page struct{ Data challenges.RunPage }
	r := f.privateRequest(t, a, "GET", "/challenges/runs", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &page) != nil || len(page.Data.Items) != 2 {
		t.Fatal(r.status, string(r.body))
	}
	if r := f.privateRequest(t, b, "GET", "/challenges/runs?before="+run.ID, "", nil); r.status != 404 {
		t.Fatal("foreign cursor", r.status)
	}
}
