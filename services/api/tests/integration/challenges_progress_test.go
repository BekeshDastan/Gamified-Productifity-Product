//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/focus"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/journal"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"strings"
	"testing"
	"time"
)

func TestChallengeAutomaticFocusHabitReviewAndStages(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "challenge-sources@example.test")
	start := func(in challenges.CreateInput) string {
		run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", in))
		path := "/challenges/runs/" + run.ID
		readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}))
		return path
	}
	check := func(path string, want float64) {
		t.Helper()
		m := readChallengeMember(t, f.privateRequest(t, a, "GET", path+"/progress", "", nil))
		if p := challengeProgress(t, m); p.Current != want {
			t.Fatal(path, p, want)
		}
	}
	fp := start(challengeInput(t, 0))
	f.clock.Advance(time.Minute)
	s := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", focusInput()))
	f.clock.Advance(90 * time.Second)
	s = focusAct(t, f, a, s, "pause")
	f.clock.Advance(5 * time.Minute)
	s = focusAct(t, f, a, s, "resume")
	f.clock.Advance(30 * time.Second)
	s = focusAct(t, f, a, s, "finish")
	check(fp, 0)
	readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), focus.ConfirmInput{Result: "progress"}))
	check(fp, 2)
	s = readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", focusInput()))
	f.clock.Advance(time.Minute)
	s = focusAct(t, f, a, s, "finish")
	readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), focus.ConfirmInput{Result: "rest"}))
	check(fp, 2)
	h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", habitInput()))
	in := challengeInput(t, 4)
	in.Input.HabitID = &h.ID
	hp := start(in)
	f.clock.Advance(time.Minute)
	h = readHabit(t, f.privateRequest(t, a, "POST", "/habits/"+h.ID+"/logs", habitVersion(h), habits.LogInput{PeriodID: h.Periods[0].ID, Amount: 20, OccurredAt: f.clock.Now()}))
	check(hp, 1)
	periodPath := "/habits/" + h.ID + "/periods/" + h.Periods[0].ID
	var period struct{ Data habits.Period }
	pr := f.privateRequest(t, a, "GET", periodPath, "", nil)
	if pr.status != 200 || json.Unmarshal(pr.body, &period) != nil || period.Data.Amount != 20 {
		t.Fatal(pr.status, string(pr.body))
	}
	other, _ := f.signedIn(t, "challenge-habit-other@example.test")
	if pr := f.privateRequest(t, other, "GET", periodPath, "", nil); pr.status != 404 {
		t.Fatal("foreign period source", pr.status)
	}
	logs := readHabitLogs(t, f.privateRequest(t, a, "GET", "/habits/"+h.ID+"/logs", "", nil))
	h = readHabit(t, f.privateRequest(t, a, "POST", "/habits/"+h.ID+"/logs/"+logs[0].ID+"/correct", habitVersion(h), habits.CorrectionInput{Amount: 0, Reason: "Исправление"}))
	check(hp, 0)
	rp := start(challengeInput(t, 6))
	f.clock.Advance(time.Minute)
	review := readReview(t, f.privateRequest(t, a, "PUT", "/reviews/day/2026-10-12", `"0"`, journal.ReviewInput{Obstacle: "none", Conclusion: "PRIVATE-CHALLENGE-BODY"}))
	check(rp, 1)
	readReview(t, f.privateRequest(t, a, "PUT", "/reviews/day/2026-10-12", fmt.Sprintf(`"%d"`, review.Run.Version), journal.ReviewInput{Obstacle: "none", Conclusion: "PRIVATE-CHALLENGE-BODY-EDIT"}))
	check(rp, 1)
	r := f.privateRequest(t, a, "GET", rp+"/sources", "", nil)
	if string(r.body) == "" || strings.Contains(string(r.body), "PRIVATE-CHALLENGE") {
		t.Fatal("review body exposed")
	}
	gi := goalInput("Этапы испытания")
	gi.ProgressType = "milestones"
	g := readGoal(t, f.privateRequest(t, a, "POST", "/goals", "", goals.Create{Input: gi, State: "active"}))
	g = readGoal(t, f.privateRequest(t, a, "POST", "/goals/"+g.ID+"/milestones", goalVersion(g), goals.MilestoneInput{Title: "Результат", Required: true, Weight: 1}))
	in = challengeInput(t, 11)
	in.Input.GoalID = &g.ID
	gp := start(in)
	f.clock.Advance(time.Minute)
	var plan struct{ Data goals.Plan }
	r = f.privateRequest(t, a, "GET", "/goals/"+g.ID+"/plan", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &plan) != nil {
		t.Fatal(r.status, string(r.body))
	}
	m := plan.Data.Milestones[0]
	m.Completed = true
	g = readGoal(t, f.privateRequest(t, a, "PATCH", "/goals/"+g.ID+"/milestones/"+m.ID, goalVersion(g), m.MilestoneInput))
	check(gp, 1)
	m.Completed = false
	m.Reason = "Исправление"
	g = readGoal(t, f.privateRequest(t, a, "PATCH", "/goals/"+g.ID+"/milestones/"+m.ID, goalVersion(g), m.MilestoneInput))
	check(gp, 0)
	m.Completed = true
	readGoal(t, f.privateRequest(t, a, "PATCH", "/goals/"+g.ID+"/milestones/"+m.ID, goalVersion(g), m.MilestoneInput))
	check(gp, 1)
}

func challengeProgress(t *testing.T, m challenges.Member) challenges.Progress {
	t.Helper()
	var p challenges.Progress
	if e := json.Unmarshal(m.Progress, &p); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestChallengeAutomaticTasksCorrectionsAndWindows(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "challenge-progress@example.test")
	b, _ := f.signedIn(t, "challenge-progress-other@example.test")
	old := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("До вступления")))
	old = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+old.ID+"/complete", version(old), quests.Transition{}))
	f.clock.Advance(time.Minute)
	in := challengeInput(t, 2)
	in.Input.Rejoin = true
	run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", in))
	path := "/challenges/runs/" + run.ID
	readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}))
	progress := func() challenges.Member {
		return readChallengeMember(t, f.privateRequest(t, a, "GET", path+"/progress", "", nil))
	}
	if p := challengeProgress(t, progress()); p.Current != 0 {
		t.Fatal(p)
	}
	f.clock.Advance(time.Minute)
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("После вступления")))
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	first := progress()
	if p := challengeProgress(t, first); p.Current != 1 || p.SourceCount != 1 {
		t.Fatal(p)
	}
	if m := progress(); m.Version != first.Version {
		t.Fatal("duplicate changed progress version")
	}
	var sources struct{ Data challenges.SourcePage }
	r := f.privateRequest(t, a, "GET", path+"/sources", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &sources) != nil || sources.Data.Total != 1 || sources.Data.Items[0].Key != q.ID {
		t.Fatal(r.status, string(r.body))
	}
	if r := f.privateRequest(t, b, "GET", path+"/sources", "", nil); r.status != 404 {
		t.Fatal("foreign sources", r.status)
	}
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/reopen", version(q), quests.Transition{Reason: "Исправление"}))
	if p := challengeProgress(t, progress()); p.Current != 0 {
		t.Fatal("reopen", p)
	}
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	if p := challengeProgress(t, progress()); p.Current != 1 {
		t.Fatal("restore", p)
	}
	old = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+old.ID+"/reopen", version(old), quests.Transition{Reason: "Исправление"}))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+old.ID+"/complete", version(old), quests.Transition{}))
	if p := challengeProgress(t, progress()); p.Current != 1 {
		t.Fatal("old source recycled", p)
	}
	f.clock.Advance(time.Minute)
	m := progress()
	m = readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/leave", fmt.Sprintf(`"%d"`, m.Version), challenges.LeaveInput{}))
	f.clock.Advance(time.Minute)
	outside := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Вне участия")))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+outside.ID+"/complete", version(outside), quests.Transition{}))
	f.clock.Advance(time.Minute)
	readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true, MembershipVersion: m.Version}))
	if p := challengeProgress(t, progress()); p.Current != 1 {
		t.Fatal("outside window", p)
	}
	if _, e := f.pool.Exec(f.ctx, "DELETE FROM challenge_source_credits WHERE owner_id=$1", who.ID); e != nil {
		t.Fatal(e)
	}
	if p := challengeProgress(t, progress()); p.Current != 1 {
		t.Fatal("rebuild", p)
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM reward_entitlements WHERE owner_id=$1 AND source_kind='challenge'", who.ID).Scan(&n); e != nil || n != 0 {
		t.Fatal("progress awarded premature XP", n, e)
	}
}
