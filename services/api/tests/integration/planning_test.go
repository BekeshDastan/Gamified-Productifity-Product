//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"testing"
	"time"
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

func TestPlanningPrioritiesMultipleBlocksAndDeadline(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "blocks-plan@example.test")
	qIn := questInput("Два блока работы")
	deadline := f.clock.Now().Add(72 * time.Hour)
	qIn.Deadline = &deadline
	q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", qIn))
	in := planInput()
	in.QuestIDs = []string{q.ID}
	in.TopIDs = []string{q.ID}
	path := "/plans/day/2026-10-13"
	p := readPlan(t, f.privateRequest(t, client, "PUT", path, `"0"`, in))
	first := planning.Block{Move: planning.Move{QuestID: q.ID, QuestVersion: q.Version, Reason: "Утренний блок"}, Start: "09:00", End: "10:00"}
	readEvent(t, f.privateRequest(t, client, "POST", path+"/blocks", planVersion(p), first, "planning-first-block"))
	readEvent(t, f.privateRequest(t, client, "POST", path+"/blocks", planVersion(p), first, "planning-first-block"))
	p = readPlan(t, f.privateRequest(t, client, "GET", path, "", nil))
	q = readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+q.ID, "", nil))
	if q.PlannedDate == nil || *q.PlannedDate != "2026-10-13" || !q.Deadline.Equal(deadline) {
		t.Fatal("date or deadline changed", q)
	}
	second := first
	second.QuestVersion = q.Version
	second.Start = "14:00"
	second.End = "15:00"
	readEvent(t, f.privateRequest(t, client, "POST", path+"/blocks", planVersion(p), second))
	p = readPlan(t, f.privateRequest(t, client, "GET", path, "", nil))
	p = readPlan(t, f.privateRequest(t, client, "POST", path+"/confirm", planVersion(p), struct{}{}))
	if len(p.Snapshot.Blocks) != 2 {
		t.Fatal("blocks lost or duplicated", len(p.Snapshot.Blocks))
	}
	var ids []string
	for i := 0; i < 4; i++ {
		q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput(fmt.Sprintf("Приоритет %d", i))))
		ids = append(ids, q.ID)
	}
	in.QuestIDs = ids
	in.TopIDs = ids
	if r := f.privateRequest(t, client, "PUT", "/plans/day/2026-10-14", `"0"`, in); r.status != 422 {
		t.Fatal("four priorities accepted", r.status)
	}
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

func TestPlanningFrozenSnapshotAndAddenda(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "snapshot-plan@example.test")
	a := questInput("Важное действие")
	a.Priority = "high"
	a.EstimatedMinutes = 90
	q1 := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", a))
	a.Title = "Малое действие"
	a.Priority = "low"
	a.EstimatedMinutes = 30
	q2 := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", a))
	in := planInput()
	in.QuestIDs = []string{q1.ID, q2.ID}
	in.TopIDs = []string{q1.ID}
	path := "/plans/day/2026-10-12"
	p := readPlan(t, f.privateRequest(t, client, "PUT", path, `"0"`, in))
	p = readPlan(t, f.privateRequest(t, client, "POST", path+"/confirm", planVersion(p), struct{}{}))
	if p.Snapshot == nil || p.Progress.TotalWeight != 4 || p.Progress.Committed != 2 || p.Progress.Score == nil || *p.Progress.Score != 0 {
		t.Fatal("snapshot missing", p)
	}
	if r := f.privateRequest(t, client, "PUT", path, planVersion(p), planInput()); r.status != 409 {
		t.Fatal("confirmed overwritten", r.status)
	}
	in.QuestIDs = []string{q1.ID}
	p = readPlan(t, f.privateRequest(t, client, "POST", path+"/addendum", planVersion(p), planning.Amendment{Input: in, Reason: "Меньше времени"}))
	if len(p.Snapshot.Items) != 2 || p.Progress.TotalWeight != 4 || len(p.QuestIDs) != 1 || len(p.Changes) != 2 {
		t.Fatal("denominator changed", p)
	}
	readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+q1.ID+"/complete", version(q1), map[string]string{"reason": "Готово"}))
	p = readPlan(t, f.privateRequest(t, client, "GET", path, "", nil))
	if p.Progress.CompletedWeight != 3 || *p.Progress.Score != 75 {
		t.Fatal("weighted snapshot", p.Progress)
	}
	f.clock.Advance(24 * 60 * 60 * 1e9)
	if r := f.request(t, client, "POST", "/refresh", f.csrf(t, client), struct{}{}); r.status != 200 {
		t.Fatal("refresh after advancing clock", r.status)
	}
	readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+q2.ID+"/complete", version(q2), map[string]string{"reason": "На следующий день"}))
	p = readPlan(t, f.privateRequest(t, client, "GET", path, "", nil))
	if *p.Progress.Score != 75 {
		t.Fatal("late completion changed prior day")
	}
	rest := planning.Input{Rest: true}
	r := readPlan(t, f.privateRequest(t, client, "PUT", "/plans/day/2026-10-14", `"0"`, rest))
	r = readPlan(t, f.privateRequest(t, client, "POST", "/plans/day/2026-10-14/confirm", planVersion(r), struct{}{}))
	if r.Progress.Score != nil {
		t.Fatal("rest scored")
	}
}
