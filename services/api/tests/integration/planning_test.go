//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"testing"
)

func readPlan(t *testing.T, r authResponse) planning.Plan {
	t.Helper()
	var out struct {
		Data planning.Plan `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatalf("plan %d %s", r.status, r.body)
	}
	return out.Data
}
func planInput() planning.Input {
	return planning.Input{AvailableMinutes: 120, Windows: []planning.Window{{Start: "09:00", End: "12:00"}}, QuestIDs: []string{}, TopIDs: []string{}}
}
func planVersion(p planning.Plan) string { return fmt.Sprintf(`"%d"`, p.Version) }
func TestPlanningDraftOwnershipAndVersion(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "plan@example.test")
	other, _ := f.signedIn(t, "other-plan@example.test")
	path := "/plans/day/2026-10-12"
	empty := readPlan(t, f.privateRequest(t, client, "GET", path, "", nil))
	if empty.Version != 0 || empty.ConfirmedAt != nil {
		t.Fatal("invented commitment")
	}
	p := readPlan(t, f.privateRequest(t, client, "PUT", path, `"0"`, planInput()))
	if p.Version != 1 || p.ID == "" || p.Snapshot != nil {
		t.Fatal("draft", p)
	}
	if r := f.privateRequest(t, client, "PUT", path, `"0"`, planInput()); r.status != 409 {
		t.Fatal("duplicate overwrite", r.status)
	}
	empty = readPlan(t, f.privateRequest(t, other, "GET", path, "", nil))
	if empty.ID != "" {
		t.Fatal("owner leak")
	}
	var n int
	if e := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM day_plans`).Scan(&n); e != nil || n != 1 {
		t.Fatal("duplicate plan", n, e)
	}
	in := planInput()
	in.TopIDs = []string{"00000000-0000-4000-8000-000000000000"}
	if r := f.privateRequest(t, client, "PUT", path, planVersion(p), in); r.status != 422 {
		t.Fatal("invalid top accepted", r.status)
	}
}
