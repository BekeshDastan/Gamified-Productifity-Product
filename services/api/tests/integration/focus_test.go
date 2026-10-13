//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/focus"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
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
