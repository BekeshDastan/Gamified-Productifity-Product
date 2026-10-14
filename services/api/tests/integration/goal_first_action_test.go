//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"testing"
)

func TestGoalFirstActionConfirmedAtomicVersionAndOwner(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "first-action@example.test")
	b, _ := f.signedIn(t, "first-action-other@example.test")
	g := readGoal(t, f.privateRequest(t, a, "POST", "/goals", "", goals.Create{Input: goalInput("Результат"), State: "active"}))
	in := map[string]any{"task": questInput("Первый шаг"), "confirm": false}
	path := "/navigator/goals/" + g.ID + "/first-action"
	if r := f.privateRequest(t, a, "POST", path, goalVersion(g), in); r.status != 422 {
		t.Fatal("consent ignored", r.status)
	}
	var count int
	f.pool.QueryRow(f.ctx, `SELECT count(*) FROM quests WHERE owner_id=$1`, who.ID).Scan(&count)
	if count != 0 {
		t.Fatal("unconfirmed step created")
	}
	in["confirm"] = true
	if r := f.privateRequest(t, b, "POST", path, goalVersion(g), in); r.status != 404 {
		t.Fatal("foreign", r.status)
	}
	if r := f.privateRequest(t, a, "POST", path, `"99"`, in); r.status != 409 {
		t.Fatal("stale", r.status)
	}
	r := f.privateRequest(t, a, "POST", path, goalVersion(g), in, "first-action-once-key")
	var out struct{ Data goals.FirstActionResult }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	again := f.privateRequest(t, a, "POST", path, goalVersion(g), in, "first-action-once-key")
	if again.status != 200 {
		t.Fatal("replay", again.status)
	}
	f.pool.QueryRow(f.ctx, `SELECT count(*) FROM quests WHERE owner_id=$1`, who.ID).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate step", count)
	}
	if r := f.privateRequest(t, a, "POST", path, goalVersion(out.Data.Goal), in); r.status != 409 {
		t.Fatal("second first action", r.status)
	}
	var links int
	f.pool.QueryRow(f.ctx, `SELECT count(*) FROM goal_quests WHERE owner_id=$1 AND goal_id=$2 AND quest_id=$3`, who.ID, g.ID, out.Data.Task.ID).Scan(&links)
	if links != 1 {
		t.Fatal("not attached atomically")
	}
}
