//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/rewards"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/seasons"
	"testing"
)

func readSeasonOverview(t *testing.T, r authResponse) seasons.Overview {
	t.Helper()
	var out struct{ Data seasons.Overview }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestSeasonLinksKeepUnitsBaselinesAndOwnedLiveSources(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "season-links@example.test")
	b, _ := f.signedIn(t, "season-links-other@example.test")
	season := readSeason(t, f.privateRequest(t, a, "POST", "/seasons", "", seasonInput()))
	path := "/seasons/" + season.ID
	gi := goalInput("Прочитать материал")
	gi.ProgressType = "metric"
	gi.Target = 20
	gi.Unit = "страниц"
	g := readGoal(t, f.privateRequest(t, a, "POST", "/goals", "", goals.Create{Input: gi, State: "active"}))
	foreign := readGoal(t, f.privateRequest(t, b, "POST", "/goals", "", goals.Create{Input: goalInput("Чужая цель"), State: "active"}))
	run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", challengeInput(t, 2)))
	readChallengeMember(t, f.privateRequest(t, a, "POST", "/challenges/runs/"+run.ID+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}))
	links := seasons.LinksInput{GoalIDs: []string{g.ID}, MainGoalID: &g.ID, ChallengeIDs: []string{run.ID}}
	out := readSeasonOverview(t, f.privateRequest(t, a, "PUT", path+"/links", `"1"`, links))
	if out.MainGoalID == nil || *out.MainGoalID != g.ID || len(out.Goals) != 1 || len(out.Challenges) != 1 || out.Goals[0].Unit != "страниц" || out.Challenges[0].Unit != "tasks" {
		t.Fatal(out)
	}
	var baseline seasons.Source
	if json.Unmarshal(out.Goals[0].Baseline, &baseline) != nil || baseline.Value == nil || *baseline.Value != 0 {
		t.Fatal("baseline", out)
	}
	g = readGoal(t, f.privateRequest(t, a, "POST", "/goals/"+g.ID+"/entries", goalVersion(g), goals.MetricInput{Value: 5, Unit: "страниц", OccurredOn: "2026-10-12"}))
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Подтверждённый шаг")))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	out = readSeasonOverview(t, f.privateRequest(t, a, "GET", path+"/overview", "", nil))
	if out.Goals[0].Value == nil || *out.Goals[0].Value != 5 || out.Goals[0].Target != 20 || out.Challenges[0].Value == nil || *out.Challenges[0].Value != 1 || out.Goals[0].Unit == out.Challenges[0].Unit {
		t.Fatal("mixed or stale sources", out)
	}
	for range 2 {
		readSeasonOverview(t, f.privateRequest(t, a, "GET", path+"/overview", "", nil))
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM reward_entitlements WHERE owner_id=$1 AND source_kind='challenge'", who.ID).Scan(&n); e != nil || n != 0 {
		t.Fatal("season link granted XP", n, e)
	}
	bad := links
	bad.GoalIDs = []string{foreign.ID}
	bad.MainGoalID = &foreign.ID
	if r := f.privateRequest(t, a, "PUT", path+"/links", `"2"`, bad); r.status != 404 {
		t.Fatal("foreign source", r.status)
	}
	if r := f.privateRequest(t, b, "GET", path+"/overview", "", nil); r.status != 404 {
		t.Fatal("foreign overview", r.status)
	}
	if r := f.privateRequest(t, a, "PUT", path+"/links", `"1"`, links); r.status != 409 {
		t.Fatal("stale links", r.status)
	}
	out = readSeasonOverview(t, f.privateRequest(t, a, "PUT", path+"/links", `"2"`, links))
	if json.Unmarshal(out.Goals[0].Baseline, &baseline) != nil || *baseline.Value != 0 {
		t.Fatal("baseline rewritten")
	}
	g = readGoal(t, f.privateRequest(t, a, "POST", "/goals/"+g.ID+"/cancel", goalVersion(g), goals.Transition{Reason: "Изменил направление"}))
	readGoal(t, f.privateRequest(t, a, "POST", "/goals/"+g.ID+"/archive", goalVersion(g), goals.Transition{Reason: "Сохраняю историю"}))
	out = readSeasonOverview(t, f.privateRequest(t, a, "GET", path+"/overview", "", nil))
	if out.Goals[0].State != "unavailable" || out.Goals[0].Value != nil || out.Goals[0].Percent != nil {
		t.Fatal("missing data became score", out)
	}
	out = readSeasonOverview(t, f.privateRequest(t, a, "PUT", path+"/links", fmt.Sprintf(`"%d"`, out.Season.Version), seasons.LinksInput{}))
	if len(out.Goals) != 0 || len(out.Challenges) != 0 || out.MainGoalID != nil {
		t.Fatal("empty replacement retained links", out)
	}
}
func TestSeasonLinksActualMonthlyBossAndDetectsSelectionChange(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "season-monthly@example.test")
	season := readSeason(t, f.privateRequest(t, a, "POST", "/seasons", "", seasonInput()))
	path := "/seasons/" + season.ID
	g := readGoal(t, f.privateRequest(t, a, "POST", "/goals", "", goals.Create{Input: goalInput("Результат месяца"), State: "active"}))
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Шаг босса")))
	g = readGoal(t, f.privateRequest(t, a, "PUT", "/goals/"+g.ID+"/links/"+q.ID, goalVersion(g), goals.LinkInput{Required: true, Weight: 1}))
	boss := readBoss(t, f.privateRequest(t, a, "POST", "/bosses", "", map[string]string{"goal_id": g.ID}))
	out := readSeasonOverview(t, f.privateRequest(t, a, "PUT", path+"/links", `"1"`, seasons.LinksInput{GoalIDs: []string{g.ID}, MainGoalID: &g.ID}))
	var monthly struct{ Data rewards.MonthlyBoss }
	r := f.privateRequest(t, a, "GET", "/bosses/monthly", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &monthly) != nil {
		t.Fatal(r.status, string(r.body))
	}
	r = f.privateRequest(t, a, "PUT", "/bosses/monthly", fmt.Sprintf(`"%d"`, monthly.Data.Version), map[string]any{"month": monthly.Data.Month, "boss_id": boss.ID, "reason": "Результат месяца"})
	if r.status != 200 || json.Unmarshal(r.body, &monthly) != nil {
		t.Fatal(r.status, string(r.body))
	}
	link := seasons.MonthlyLinkInput{BossID: boss.ID, Month: monthly.Data.Month, SourceVersion: monthly.Data.Version, Confirm: true}
	out = readSeasonOverview(t, f.privateRequest(t, a, "PUT", path+"/monthly-boss", fmt.Sprintf(`"%d"`, out.Season.Version), link))
	if len(out.Bosses) != 1 || out.Bosses[0].BossID != boss.ID || out.Bosses[0].SourceChanged {
		t.Fatal(out)
	}
	r = f.privateRequest(t, a, "PUT", "/bosses/monthly", fmt.Sprintf(`"%d"`, monthly.Data.Version), map[string]any{"month": monthly.Data.Month, "boss_id": nil, "reason": "Изменил приоритет"})
	if r.status != 200 {
		t.Fatal(r.status, string(r.body))
	}
	out = readSeasonOverview(t, f.privateRequest(t, a, "GET", path+"/overview", "", nil))
	if !out.Bosses[0].SourceChanged {
		t.Fatal("monthly change hidden")
	}
	if r := f.privateRequest(t, a, "PUT", path+"/monthly-boss", fmt.Sprintf(`"%d"`, out.Season.Version), link); r.status != 409 {
		t.Fatal("stale monthly source", r.status)
	}
}
