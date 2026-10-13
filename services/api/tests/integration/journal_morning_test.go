//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/journal"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"testing"
)

func readMorning(t *testing.T, r authResponse) journal.Morning {
	t.Helper()
	var out struct{ Data journal.Morning }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestJournalMorningReusesPlanAndRetainsOriginalSnapshot(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "morning-journal@example.test")
	b, _ := f.signedIn(t, "morning-other@example.test")
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Главный шаг")))
	path := "/routine-runs/morning/2026-10-12"
	initial := readMorning(t, f.privateRequest(t, a, "GET", path, "", nil))
	if initial.Run != nil || initial.Plan.Version != 0 {
		t.Fatal(initial)
	}
	in := journal.MorningInput{Input: planning.Input{AvailableMinutes: 60, QuestIDs: []string{q.ID}, TopIDs: []string{q.ID}}}
	saved := readMorning(t, f.privateRequest(t, a, "PUT", path, `"0"`, in))
	if saved.Run == nil || saved.Plan.ConfirmedAt == nil || saved.Plan.Snapshot == nil {
		t.Fatal(saved)
	}
	in.PlanVersion = saved.Plan.Version
	in.Input.AvailableMinutes = 90
	in.Reason = "Уточнил доступное время"
	next := readMorning(t, f.privateRequest(t, a, "PUT", path, fmt.Sprintf(`"%d"`, saved.Run.Version), in))
	if next.Run.ID != saved.Run.ID || next.Plan.ID != saved.Plan.ID || next.Plan.AvailableMinutes != 90 || next.Plan.Snapshot.Input.AvailableMinutes != 60 {
		t.Fatal(next)
	}
	if r := f.privateRequest(t, a, "PUT", path, `"1"`, in); r.status != 409 {
		t.Fatal("stale run", r.status)
	}
	foreign := readMorning(t, f.privateRequest(t, b, "GET", path, "", nil))
	if foreign.Run != nil || foreign.Plan.ID != "" {
		t.Fatal("foreign", foreign)
	}
	var n int
	for _, table := range []string{"routine_runs", "day_plans"} {
		if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table+" WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 1 {
			t.Fatal(table, n, e)
		}
	}
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM routine_run_events WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 2 {
		t.Fatal(n, e)
	}
}
