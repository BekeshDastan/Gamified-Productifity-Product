//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/character"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/seasons"
	"sync"
	"testing"
	"time"
)

func readSeasonSummary(t *testing.T, r authResponse) seasons.Summary {
	t.Helper()
	var out struct{ Data seasons.Summary }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestSeasonClosePreservesSourceSnapshotLevelAndArchive(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "season-close@example.test")
	old := questInput("До начала сезона")
	old.Difficulty = 5
	q0 := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", old))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q0.ID+"/complete", version(q0), quests.Transition{}))
	f.clock.Advance(time.Minute)
	v := readSeason(t, f.privateRequest(t, a, "POST", "/seasons", "", seasonInput()))
	path := "/seasons/" + v.ID
	v = readSeason(t, f.privateRequest(t, a, "POST", path+"/activate", `"1"`, seasons.Confirmation{Confirm: true}))
	g := readGoal(t, f.privateRequest(t, a, "POST", "/goals", "", goals.Create{Input: goalInput("Результат сезона"), State: "active"}))
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("В сезон")))
	g = readGoal(t, f.privateRequest(t, a, "PUT", "/goals/"+g.ID+"/links/"+q.ID, goalVersion(g), goals.LinkInput{Required: true, Weight: 1}))
	out := readSeasonOverview(t, f.privateRequest(t, a, "PUT", path+"/links", fmt.Sprintf(`"%d"`, v.Version), seasons.LinksInput{GoalIDs: []string{g.ID}, MainGoalID: &g.ID}))
	f.clock.Advance(time.Minute)
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	f.clock.Advance(time.Minute)
	if r := f.privateRequest(t, a, "POST", path+"/close", fmt.Sprintf(`"%d"`, out.Season.Version), seasons.CloseInput{Confirm: true}); r.status != 422 {
		t.Fatal("early close without reason", r.status)
	}
	var before int64
	if e := f.pool.QueryRow(f.ctx, "SELECT total_xp FROM user_progress WHERE owner_id=$1", who.ID).Scan(&before); e != nil {
		t.Fatal(e)
	}
	summary := readSeasonSummary(t, f.privateRequest(t, a, "POST", path+"/close", fmt.Sprintf(`"%d"`, out.Season.Version), seasons.CloseInput{Confirm: true, Reason: "Сохраняю промежуточный итог"}))
	if summary.Overview.Season.State != "completed" || !summary.Early || summary.Activity.Tasks != 1 || summary.Activity.XPDelta != 10 || summary.Activity.LevelAtClose < 2 || *summary.Overview.Goals[0].Value != 100 {
		t.Fatal(summary)
	}
	var after int64
	if e := f.pool.QueryRow(f.ctx, "SELECT total_xp FROM user_progress WHERE owner_id=$1", who.ID).Scan(&after); e != nil || before != after {
		t.Fatal("season reset or granted XP", before, after, e)
	}
	closed := readSeason(t, f.privateRequest(t, a, "GET", path, "", nil))
	snapshot := append([]byte(nil), closed.FinalSnapshot...)
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/reopen", version(q), quests.Transition{Reason: "Исправление после сезона"}))
	frozen := readSeasonOverview(t, f.privateRequest(t, a, "GET", path+"/overview", "", nil))
	if frozen.Goals[0].Value == nil || *frozen.Goals[0].Value != 100 {
		t.Fatal("past summary changed", frozen)
	}
	repeat := readSeasonSummary(t, f.privateRequest(t, a, "POST", path+"/close", `"1"`, seasons.CloseInput{Confirm: true, Reason: "Повтор"}))
	if !repeat.At.Equal(summary.At) || repeat.Reason != summary.Reason {
		t.Fatal("close replaced original")
	}
	archived := readSeason(t, f.privateRequest(t, a, "POST", path+"/archive", fmt.Sprintf(`"%d"`, closed.Version), seasons.ArchiveInput{Confirm: true, Archived: true}))
	if archived.ArchivedAt == nil || !bytes.Equal(snapshot, archived.FinalSnapshot) {
		t.Fatal("archive rewrote report")
	}
	readSeasonSummary(t, f.privateRequest(t, a, "GET", path+"/summary", "", nil))
	next := readSeason(t, f.privateRequest(t, a, "POST", "/seasons", "", seasonInput()))
	readSeason(t, f.privateRequest(t, a, "POST", "/seasons/"+next.ID+"/activate", `"1"`, seasons.Confirmation{Confirm: true}))
	var achievements struct{ Data []character.AchievementStatus }
	r := f.privateRequest(t, a, "GET", "/achievements", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &achievements) != nil {
		t.Fatal(r.status, string(r.body))
	}
	found := false
	for _, d := range achievements.Data {
		if d.ID == "season-1" {
			found = true
			if !d.Active || d.Current != 1 {
				t.Fatal("season achievement", d)
			}
		}
	}
	if !found {
		t.Fatal("season achievement missing")
	}
	if _, e := f.pool.Exec(f.ctx, "UPDATE personal_seasons SET final_snapshot='{}' WHERE owner_id=$1 AND id=$2", who.ID, v.ID); e == nil {
		t.Fatal("mutable first summary")
	}
}
func TestSeasonDeadlineWorkerAndAPICloseAreOneResult(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "season-race@example.test")
	in := seasonInput()
	in.EndsOn = in.StartsOn
	v := readSeason(t, f.privateRequest(t, a, "POST", "/seasons", "", in))
	path := "/seasons/" + v.ID
	v = readSeason(t, f.privateRequest(t, a, "POST", path+"/activate", `"1"`, seasons.Confirmation{Confirm: true}))
	f.clock.Advance(12 * time.Hour)
	f.loginAs(t, a, "season-race@example.test")
	store := jobs.Store{Pool: f.pool, Clock: f.clock}
	var deadline jobs.Job
	for i := 0; i < 100; i++ {
		job, e := store.Claim(f.ctx)
		if e != nil {
			t.Fatal(e)
		}
		if job.Kind == "seasons.close" {
			deadline = job
			break
		}
	}
	if deadline.ID == "" {
		t.Fatal("season deadline missing")
	}
	service := seasons.NewService(f.pool, f.clock)
	var wg sync.WaitGroup
	var workerErr error
	responses := make([]authResponse, 2)
	wg.Add(3)
	go func() { defer wg.Done(); workerErr = store.Process(f.ctx, deadline, service.Handlers()) }()
	for i := range responses {
		go func(i int) {
			defer wg.Done()
			responses[i] = f.privateRequest(t, a, "POST", path+"/close", fmt.Sprintf(`"%d"`, v.Version), seasons.CloseInput{Confirm: true})
		}(i)
	}
	wg.Wait()
	if workerErr != nil {
		t.Fatal(workerErr)
	}
	first := readSeasonSummary(t, responses[0])
	second := readSeasonSummary(t, responses[1])
	if first.Early || !first.At.Equal(second.At) || first.Overview.Season.State != "completed" {
		t.Fatal(first, second)
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM personal_season_events WHERE owner_id=$1 AND kind='closed'", who.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if e := store.Process(f.ctx, deadline, service.Handlers()); e != jobs.ErrLeaseLost {
		t.Fatal("replayed deadline", e)
	}
}
