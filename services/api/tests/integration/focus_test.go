//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/focus"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"net/http"
	"sync"
	"testing"
	"time"
)

func readFocus(t *testing.T, r authResponse) focus.Session {
	t.Helper()
	var out struct{ Data focus.Session }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func readCurrentFocus(t *testing.T, r authResponse) focus.Current {
	t.Helper()
	var out struct{ Data focus.Current }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func focusVersion(f focus.Session) string { return fmt.Sprintf(`"%d"`, f.Version) }
func focusInput() focus.StartInput {
	return focus.StartInput{OperationID: testkit.NewUser().ID, Mode: "free", WorkMinutes: 25, BreakMinutes: 5}
}
func TestFocusStartRaceServerTimingAndPrivacy(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "focus-race@example.test")
	other, _ := f.signedIn(t, "focus-private@example.test")
	inputs := []focus.StartInput{focusInput(), focusInput()}
	results := make([]authResponse, 2)
	var wg sync.WaitGroup
	for i := range inputs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = f.privateRequest(t, a, "POST", "/focus/start", "", inputs[i])
		}(i)
	}
	wg.Wait()
	winner := -1
	for i, r := range results {
		if r.status == 200 {
			if winner >= 0 {
				t.Fatal("two starts")
			}
			winner = i
		} else if r.status != 409 {
			t.Fatal(r.status, string(r.body))
		}
	}
	if winner < 0 {
		t.Fatal("no start")
	}
	session := readFocus(t, results[winner])
	if !session.StartedAt.Equal(f.clock.Now()) || session.ActiveSeconds != 0 || len(session.Segments) != 1 {
		t.Fatal(session)
	}
	f.clock.Advance(90 * time.Second)
	cur := readCurrentFocus(t, f.privateRequest(t, a, "GET", "/focus/current", "", nil))
	if cur.Session == nil || cur.Session.ID != session.ID || cur.Session.ActiveSeconds != 90 || !cur.ServerNow.Equal(f.clock.Now()) {
		t.Fatal(cur)
	}
	replay := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", inputs[winner]))
	if replay.ID != session.ID {
		t.Fatal("duplicate replay")
	}
	foreign := readCurrentFocus(t, f.privateRequest(t, other, "GET", "/focus/current", "", nil))
	if foreign.Session != nil {
		t.Fatal("privacy")
	}
	var n int
	if e := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM focus_sessions WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	q := readQuest(t, f.privateRequest(t, other, "POST", "/quests", "", questInput("Чужая задача")))
	in := focusInput()
	in.QuestID = &q.ID
	if r := f.privateRequest(t, a, "POST", "/focus/start", "", in); r.status != 409 {
		t.Fatal("active precedence", r.status)
	}
	in = focusInput()
	in.QuestID = &session.ID
	if r := f.privateRequest(t, other, "POST", "/focus/start", "", in); r.status != 404 {
		t.Fatal("foreign quest", r.status, string(r.body))
	}
}
func focusAct(t *testing.T, f authFixture, a *http.Client, s focus.Session, action string) focus.Session {
	return readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/actions", focusVersion(s), focus.ActionInput{Action: action}))
}
func TestFocusPauseResumeFinishAndPermanentReplay(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "focus-time@example.test")
	other, _ := f.signedIn(t, "focus-time-other@example.test")
	in := focusInput()
	s := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", in))
	f.clock.Advance(90 * time.Second)
	s = focusAct(t, f, a, s, "pause")
	if s.State != "paused" || s.ActiveSeconds != 90 {
		t.Fatal(s)
	}
	f.clock.Advance(time.Hour)
	f.loginAs(t, a, "focus-time@example.test")
	repeated := focusAct(t, f, a, s, "pause")
	if repeated.Version != s.Version || repeated.ActiveSeconds != 90 {
		t.Fatal("repeat pause", repeated)
	}
	old := s
	s = focusAct(t, f, a, s, "resume")
	if r := f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/actions", focusVersion(old), focus.ActionInput{Action: "pause"}); r.status != 409 {
		t.Fatal("stale", r.status)
	}
	f.clock.Advance(30 * time.Second)
	s = focusAct(t, f, a, s, "finish")
	if s.State != "awaiting_confirmation" || s.ActiveSeconds != 120 || s.EndedAt == nil || len(s.Segments) != 2 {
		t.Fatal(s)
	}
	f.clock.Advance(time.Hour)
	f.loginAs(t, a, "focus-time@example.test")
	if again := focusAct(t, f, a, s, "finish"); again.ActiveSeconds != 120 || again.Version != s.Version {
		t.Fatal("repeat finish", again)
	}
	f.loginAs(t, other, "focus-time-other@example.test")
	if r := f.privateRequest(t, other, "POST", "/focus/sessions/"+s.ID+"/actions", focusVersion(s), focus.ActionInput{Action: "cancel"}); r.status != 404 {
		t.Fatal("foreign", r.status)
	}
	s = focusAct(t, f, a, s, "cancel")
	if r := f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/actions", focusVersion(s), focus.ActionInput{Action: "finish"}); r.status != 409 {
		t.Fatal("finish cancelled", r.status)
	}
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "focus-time@example.test")
	replay := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", in))
	if replay.ID != s.ID || replay.State != "cancelled" {
		t.Fatal("expired replay", replay)
	}
	var n int
	if e := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM reward_ledger WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 0 {
		t.Fatal("timer XP", n, e)
	}
	if _, e := f.pool.Exec(t.Context(), "UPDATE focus_segments SET ended_at=ended_at+interval '1 hour' WHERE owner_id=$1", who.ID); e == nil {
		t.Fatal("timing mutable")
	}
}
func TestFocusFourHourWorkAndEightHourWallLimits(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "focus-limits@example.test")
	s := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", focusInput()))
	started := s.StartedAt
	f.clock.Advance(5 * time.Hour)
	f.loginAs(t, a, "focus-limits@example.test")
	cur := readCurrentFocus(t, f.privateRequest(t, a, "GET", "/focus/current", "", nil))
	s = *cur.Session
	if s.State != "awaiting_confirmation" || s.ActiveSeconds != 14400 || s.EndedAt == nil || !s.EndedAt.Equal(started.Add(4*time.Hour)) {
		t.Fatal("active limit", s)
	}
	f.clock.Advance(time.Hour)
	f.loginAs(t, a, "focus-limits@example.test")
	cur = readCurrentFocus(t, f.privateRequest(t, a, "GET", "/focus/current", "", nil))
	if cur.Session.ActiveSeconds != 14400 || cur.Session.Version != s.Version {
		t.Fatal("unbounded time", cur)
	}
	focusAct(t, f, a, s, "cancel")
	s = readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", focusInput()))
	started = s.StartedAt
	f.clock.Advance(time.Minute)
	s = focusAct(t, f, a, s, "pause")
	f.clock.Advance(9 * time.Hour)
	f.loginAs(t, a, "focus-limits@example.test")
	cur = readCurrentFocus(t, f.privateRequest(t, a, "GET", "/focus/current", "", nil))
	s = *cur.Session
	if s.State != "awaiting_confirmation" || s.ActiveSeconds != 60 || s.EndedAt == nil || !s.EndedAt.Equal(started.Add(8*time.Hour)) {
		t.Fatal("wall limit", s)
	}
}
func TestFocusPomodoroPauseBreakAndExplicitNext(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "focus-pomo@example.test")
	in := focusInput()
	in.Mode = "pomodoro"
	in.WorkMinutes = 5
	in.BreakMinutes = 1
	in.SoundEnabled = true
	s := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", in))
	if s.SoundEnabled || s.PhaseDeadline == nil {
		t.Fatal("global sound off", s)
	}
	f.clock.Advance(2 * time.Minute)
	s = focusAct(t, f, a, s, "pause")
	if s.ActiveSeconds != 120 || s.PhaseRemainingSeconds == nil || *s.PhaseRemainingSeconds != 180 {
		t.Fatal(s)
	}
	f.clock.Advance(10 * time.Minute)
	s = focusAct(t, f, a, s, "resume")
	f.clock.Advance(4 * time.Minute)
	f.loginAs(t, a, "focus-pomo@example.test")
	cur := readCurrentFocus(t, f.privateRequest(t, a, "GET", "/focus/current", "", nil))
	s = *cur.Session
	if s.State != "paused" || s.Phase != "work_complete" || s.ActiveSeconds != 300 || s.Cycle != 1 {
		t.Fatal("work boundary", s)
	}
	f.clock.Advance(2 * time.Minute)
	cur = readCurrentFocus(t, f.privateRequest(t, a, "GET", "/focus/current", "", nil))
	if cur.Session.ActiveSeconds != 300 {
		t.Fatal("auto next", cur)
	}
	s = focusAct(t, f, a, s, "break")
	f.clock.Advance(2 * time.Minute)
	cur = readCurrentFocus(t, f.privateRequest(t, a, "GET", "/focus/current", "", nil))
	s = *cur.Session
	if s.State != "paused" || s.Phase != "break_complete" || s.ActiveSeconds != 300 || s.BreakSeconds != 60 {
		t.Fatal("break counted as work", s)
	}
	s = focusAct(t, f, a, s, "next")
	if s.Cycle != 2 || s.State != "running" || s.Phase != "work" {
		t.Fatal(s)
	}
	f.clock.Advance(time.Minute)
	s = focusAct(t, f, a, s, "finish")
	if s.ActiveSeconds != 360 || s.BreakSeconds != 60 {
		t.Fatal(s)
	}
}
func TestFocusQuickIntervalContinuesOnlyByChoice(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "focus-quick@example.test")
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Маленький шаг")))
	in := focusInput()
	in.Mode = "quick"
	in.WorkMinutes = 5
	in.QuestID = &q.ID
	s := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", in))
	f.clock.Advance(7 * time.Minute)
	cur := readCurrentFocus(t, f.privateRequest(t, a, "GET", "/focus/current", "", nil))
	s = *cur.Session
	if s.State != "paused" || s.Phase != "work_complete" || s.ActiveSeconds != 300 {
		t.Fatal(s)
	}
	s = focusAct(t, f, a, s, "extend")
	f.clock.Advance(2 * time.Minute)
	s = focusAct(t, f, a, s, "finish")
	if s.ActiveSeconds != 420 {
		t.Fatal(s)
	}
	q = readQuest(t, f.privateRequest(t, a, "GET", "/quests/"+q.ID, "", nil))
	if q.Status != "todo" {
		t.Fatal("auto complete", q.Status)
	}
}
