//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
)

func goalInput(title string) goals.Input {
	return goals.Input{Title: title, Criteria: "Обязательные результаты", ProgressType: "quests", Target: 1}
}
func readGoal(t *testing.T, r authResponse) goals.Goal {
	t.Helper()
	var v struct {
		Data goals.Goal `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &v) != nil || v.Data.ID == "" {
		t.Fatalf("goal %d %s", r.status, r.body)
	}
	return v.Data
}
func goalVersion(g goals.Goal) string { return fmt.Sprintf(`"%d"`, g.Version) }
func TestGoalCRUDOverdueOwnerAndPromotion(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "goal-owner@example.test")
	other, _ := f.signedIn(t, "goal-other@example.test")
	in := goalInput("Закончить проект")
	day := "2026-10-11"
	in.TargetDate = &day
	g := readGoal(t, f.privateRequest(t, client, "POST", "/goals", "", goals.Create{Input: in, State: "active"}))
	if !g.Overdue || g.State != "active" || g.Progress != 0 {
		t.Fatal("deadline changed state")
	}
	if r := f.privateRequest(t, other, "GET", "/goals/"+g.ID, "", nil); r.status != 404 {
		t.Fatal("foreign goal")
	}
	in.Title = "Новый план"
	if r := f.privateRequest(t, client, "PATCH", "/goals/"+g.ID, goalVersion(g), goals.Edit{Input: in}); r.status != 422 {
		t.Fatal("silent criteria change")
	}
	g = readGoal(t, f.privateRequest(t, client, "PATCH", "/goals/"+g.ID, goalVersion(g), goals.Edit{Input: in, Reason: "Уточнил план"}))
	if r := f.privateRequest(t, client, "PATCH", "/goals/"+g.ID, `"1"`, goals.Edit{Input: in, Reason: "Старый редактор"}); r.status != 409 {
		t.Fatal("lost update")
	}
	if r := f.privateRequest(t, client, "POST", "/goals", "", map[string]any{"owner_id": account.ID}); r.status != 422 {
		t.Fatal("owner accepted")
	}
	in.ProgressType = "metric"
	in.Unit = "книги"
	in.Start = 12
	in.Target = 10
	if r := f.privateRequest(t, client, "POST", "/goals", "", goals.Create{Input: in, State: "active"}); r.status != 422 {
		t.Fatal("decreasing metric")
	}
	if _, e := f.pool.Exec(f.ctx, `INSERT INTO onboarding_starters(owner_id,kind,title,created_at,updated_at) VALUES($1,'goal','Первая цель',$2,$2)`, account.ID, f.clock.Now()); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		r := f.privateRequest(t, client, "GET", "/goals", "", nil)
		var v struct {
			Data goals.Page `json:"data"`
		}
		if r.status != 200 || json.Unmarshal(r.body, &v) != nil || v.Data.Total != 2 {
			t.Fatalf("promotion %d %s", r.status, r.body)
		}
	}
	var count int
	if e := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM goals WHERE payload->>'title'='Первая цель'`).Scan(&count); e != nil || count != 1 {
		t.Fatal("duplicate goal draft")
	}
}
