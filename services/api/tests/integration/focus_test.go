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
func readQuestFocusTime(t *testing.T, r authResponse) focus.QuestTime {
	t.Helper()
	var out struct{ Data focus.QuestTime }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestFocusConfirmIsPermanentAndManualTimeReplacesFocus(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "focus-review@example.test")
	other, _ := f.signedIn(t, "focus-review-other@example.test")
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Один шаг")))
	in := focusInput()
	in.QuestID = &q.ID
	s := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", in))
	if r := f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), focus.ConfirmInput{Result: "progress"}); r.status != 409 {
		t.Fatal("confirm running", r.status)
	}
	f.clock.Advance(120 * time.Second)
	s = focusAct(t, f, a, s, "finish")
	timing := func() focus.QuestTime {
		return readQuestFocusTime(t, f.privateRequest(t, a, "GET", "/focus/quests/"+q.ID+"/time", "", nil))
	}
	if timing().FocusSeconds != 0 {
		t.Fatal("unconfirmed time")
	}
	confirmed := focus.ConfirmInput{Result: "progress", Comment: "Первый результат"}
	s = readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), confirmed))
	for range 3 {
		again := readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", `"1"`, confirmed))
		if again.Version != s.Version || again.ActiveSeconds != 120 {
			t.Fatal(again)
		}
	}
	if got := timing(); got.Mode != "from_focus" || got.FocusSeconds != 120 || got.EffectiveSeconds != 120 || got.ConfirmedSessions != 1 {
		t.Fatal(got)
	}
	q = readQuest(t, f.privateRequest(t, a, "GET", "/quests/"+q.ID, "", nil))
	if q.Status != "todo" || q.ActualMinutes != 0 {
		t.Fatal("timer changed task", q)
	}
	edit := q.Input
	edit.ActualMinutes = 7
	edit.ActualTimeMode = "manual_override"
	q = readQuest(t, f.privateRequest(t, a, "PATCH", "/quests/"+q.ID, version(q), edit))
	if got := timing(); got.EffectiveSeconds != 420 || got.FocusSeconds != 120 || !got.Overridden {
		t.Fatal("double time", got)
	}
	edit = q.Input
	edit.ActualTimeMode = "from_focus"
	q = readQuest(t, f.privateRequest(t, a, "PATCH", "/quests/"+q.ID, version(q), edit))
	if got := timing(); got.EffectiveSeconds != 120 || got.ManualMinutes != 7 || got.Overridden {
		t.Fatal("source restore", got)
	}
	if r := f.privateRequest(t, other, "GET", "/focus/quests/"+q.ID+"/time", "", nil); r.status != 404 {
		t.Fatal("foreign", r.status)
	}
	if r := f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), focus.ConfirmInput{Result: "rest"}); r.status != 409 {
		t.Fatal("result reroll", r.status)
	}
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "focus-review@example.test")
	replay := readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", `"1"`, confirmed))
	if replay.ActiveSeconds != 120 || replay.State != "finished" {
		t.Fatal(replay)
	}
	in = focusInput()
	in.QuestID = &q.ID
	s = readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", in))
	f.clock.Advance(time.Minute)
	s = focusAct(t, f, a, s, "finish")
	readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), focus.ConfirmInput{Result: "rest"}))
	if timing().FocusSeconds != 120 {
		t.Fatal("rest became work")
	}
	var xp, n int64
	if e := f.pool.QueryRow(t.Context(), "SELECT total_xp,(SELECT count(*) FROM reward_ledger WHERE owner_id=$1) FROM user_progress WHERE owner_id=$1", who.ID).Scan(&xp, &n); e != nil || xp != 0 || n != 0 {
		t.Fatal("timer XP", xp, n, e)
	}
}
func TestFocusConfirmedEvidenceQualifiesWithoutXP(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "focus-evidence@example.test")
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			f.loginAs(t, a, "focus-evidence@example.test")
		}
		in := focusInput()
		in.Mode = "quick"
		in.WorkMinutes = 5
		s := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", in))
		f.clock.Advance(5 * time.Minute)
		cur := readCurrentFocus(t, f.privateRequest(t, a, "GET", "/focus/current", "", nil))
		s = *cur.Session
		s = focusAct(t, f, a, s, "finish")
		readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), focus.ConfirmInput{Result: "progress"}))
	}
	p := readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
	found := false
	for _, r := range p.Records {
		if r.ID == "longest-focus" {
			if !r.Available || r.Value == nil || *r.Value != 5 || len(r.Sources) != 1 {
				t.Fatal(r)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("missing focus record")
	}
	var active bool
	var xp int64
	if e := f.pool.QueryRow(t.Context(), "SELECT active,(SELECT total_xp FROM user_progress WHERE owner_id=$1) FROM user_achievements WHERE owner_id=$1 AND definition_id='focus-10'", who.ID).Scan(&active, &xp); e != nil || !active || xp != 0 {
		t.Fatal("focus evidence", active, xp, e)
	}
}
func readFocusHistory(t *testing.T, r authResponse) focus.History {
	t.Helper()
	var out struct{ Data focus.History }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestFocusHistoryFiltersSummarySegmentsAndPrivacy(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "focus-history@example.test")
	other, _ := f.signedIn(t, "focus-history-other@example.test")
	ar := f.privateRequest(t, a, "POST", "/areas", "", map[string]any{"name": "Учёба", "color": "#6478c8", "icon": "book", "archived": false})
	var area struct{ Data struct{ ID string } }
	if ar.status != 200 || json.Unmarshal(ar.body, &area) != nil {
		t.Fatal(string(ar.body))
	}
	qi := questInput("Программа")
	qi.AreaID = &area.Data.ID
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", qi))
	in := focusInput()
	in.Mode = "pomodoro"
	in.WorkMinutes = 5
	in.BreakMinutes = 1
	in.QuestID = &q.ID
	s := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", in))
	f.clock.Advance(2 * time.Minute)
	s = focusAct(t, f, a, s, "pause")
	f.clock.Advance(time.Minute)
	s = focusAct(t, f, a, s, "resume")
	f.clock.Advance(3 * time.Minute)
	s = *readCurrentFocus(t, f.privateRequest(t, a, "GET", "/focus/current", "", nil)).Session
	s = focusAct(t, f, a, s, "break")
	f.clock.Advance(time.Minute)
	s = *readCurrentFocus(t, f.privateRequest(t, a, "GET", "/focus/current", "", nil)).Session
	s = focusAct(t, f, a, s, "next")
	f.clock.Advance(30 * time.Second)
	s = focusAct(t, f, a, s, "finish")
	first := readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), focus.ConfirmInput{Result: "progress"}))
	for i, seconds := range []int{20, 30, 40} {
		in = focusInput()
		in.QuestID = &q.ID
		s = readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", in))
		f.clock.Advance(time.Duration(seconds) * time.Second)
		if i == 0 {
			focusAct(t, f, a, s, "cancel")
		} else {
			s = focusAct(t, f, a, s, "finish")
			if i == 1 {
				readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), focus.ConfirmInput{Result: "rest"}))
			}
		}
	}
	h := readFocusHistory(t, f.privateRequest(t, a, "GET", "/focus/history?from=2026-10-12&to=2026-10-12", "", nil))
	if len(h.Items) != 4 || h.Summary.Sessions != 4 || h.Summary.WorkSeconds != 420 || h.Summary.ConfirmedSeconds != 330 || h.Summary.PendingSeconds != 40 || h.Summary.CancelledSeconds != 20 || h.Summary.RestSeconds != 30 {
		t.Fatal(h)
	}
	var sum int64
	for _, row := range h.Items {
		var work, rest int64
		for _, seg := range row.Segments {
			if seg.Kind == "work" {
				work += seg.Seconds
			} else {
				rest += seg.Seconds
			}
		}
		if row.ActiveSeconds != work || row.BreakSeconds != rest {
			t.Fatal("segment disagreement", row)
		}
		sum += work
	}
	if sum != h.Summary.WorkSeconds {
		t.Fatal(sum, h.Summary)
	}
	filtered := readFocusHistory(t, f.privateRequest(t, a, "GET", "/focus/history?from=2026-10-12&to=2026-10-12&quest_id="+q.ID+"&area_id="+area.Data.ID+"&state=finished", "", nil))
	if len(filtered.Items) != 2 || filtered.Summary.WorkSeconds != 360 {
		t.Fatal(filtered)
	}
	if empty := readFocusHistory(t, f.privateRequest(t, other, "GET", "/focus/history", "", nil)); len(empty.Items) != 0 || empty.Summary.WorkSeconds != 0 {
		t.Fatal("private", empty)
	}
	detail := readCurrentFocus(t, f.privateRequest(t, a, "GET", "/focus/sessions/"+first.ID, "", nil))
	if detail.Session == nil || detail.Session.ActiveSeconds != 330 {
		t.Fatal(detail)
	}
	for _, path := range []string{"/focus/sessions/" + first.ID, "/focus/history?before=" + first.ID} {
		if r := f.privateRequest(t, other, "GET", path, "", nil); r.status != 404 {
			t.Fatal("foreign", r.status)
		}
	}
	for _, suffix := range []string{"?from=2026-10-13&to=2026-10-12", "?from=2020-01-01&to=2026-10-12", "?before=invalid", "?state=wrong", "?quest_id=invalid"} {
		if r := f.privateRequest(t, a, "GET", "/focus/history"+suffix, "", nil); r.status != 422 {
			t.Fatal(suffix, r.status, string(r.body))
		}
	}
}
func TestFocusHistoryCursorHasNoDuplicates(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "focus-pages@example.test")
	for range 26 {
		s := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", focusInput()))
		f.clock.Advance(time.Second)
		focusAct(t, f, a, s, "cancel")
	}
	first := readFocusHistory(t, f.privateRequest(t, a, "GET", "/focus/history", "", nil))
	if len(first.Items) != 25 || first.NextCursor == nil || first.Summary.Sessions != 26 || first.Summary.CancelledSeconds != 26 {
		t.Fatal(first)
	}
	last := readFocusHistory(t, f.privateRequest(t, a, "GET", "/focus/history?before="+*first.NextCursor, "", nil))
	if len(last.Items) != 1 || last.NextCursor != nil {
		t.Fatal(last)
	}
	for _, s := range first.Items {
		if s.ID == last.Items[0].ID {
			t.Fatal("duplicate cursor")
		}
	}
}
