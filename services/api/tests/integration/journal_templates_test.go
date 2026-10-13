//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/journal"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"strings"
	"testing"
)

func readRoutineTemplate(t *testing.T, r authResponse) journal.RoutineTemplate {
	t.Helper()
	var out struct{ Data journal.RoutineTemplate }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestJournalTemplatesKeepRunSnapshotsAndOwnerHistory(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "template-journal@example.test")
	b, _ := f.signedIn(t, "template-journal-other@example.test")
	path := "/routine-runs/templates/morning"
	template := readRoutineTemplate(t, f.privateRequest(t, a, "GET", path, "", nil))
	if len(template.Steps) != 3 || template.Version != 0 {
		t.Fatal(template)
	}
	morning := readMorning(t, f.privateRequest(t, a, "PUT", "/routine-runs/morning/2026-10-12", `"0"`, journal.MorningInput{Input: planning.Input{Rest: true}, Checked: []string{"time"}}))
	if morning.Run == nil {
		t.Fatal(morning)
	}
	input := journal.TemplateInput{Steps: []journal.ChecklistStep{{ID: "confirm", Label: "Мой новый первый шаг"}, {ID: "time", Label: "Проверить время"}}}
	updated := readRoutineTemplate(t, f.privateRequest(t, a, "PUT", path, `"0"`, input))
	if updated.Version != 1 || updated.Steps[0].ID != "confirm" {
		t.Fatal(updated)
	}
	if r := f.privateRequest(t, a, "PUT", path, `"0"`, input); r.status != 409 {
		t.Fatal("stale template", r.status)
	}
	foreign := readRoutineTemplate(t, f.privateRequest(t, b, "GET", path, "", nil))
	if foreign.Version != 0 {
		t.Fatal("foreign template")
	}
	history := f.privateRequest(t, a, "GET", "/routine-runs", "", nil)
	if history.status != 200 || !strings.Contains(string(history.body), "Оценить доступное время") || strings.Contains(string(history.body), "Мой новый первый шаг") {
		t.Fatal("run rewritten", string(history.body))
	}
	if r := f.privateRequest(t, b, "GET", "/routine-runs", "", nil); r.status != 200 || strings.Contains(string(r.body), morning.Run.ID) {
		t.Fatal("foreign history", r.status)
	}
	if r := f.privateRequest(t, a, "PUT", "/routine-runs/morning/2026-10-12", `"1"`, journal.MorningInput{Input: planning.Input{Rest: true}, PlanVersion: morning.Plan.Version, Reason: "Обновление"}); r.status != 409 {
		t.Fatal("old checklist", r.status)
	}
	input.Steps[1].ID = input.Steps[0].ID
	if r := f.privateRequest(t, a, "PUT", path, `"1"`, input); r.status != 422 {
		t.Fatal("duplicate ids", r.status)
	}
	if r := f.privateRequest(t, a, "GET", "/routine-runs?before=bad", "", nil); r.status != 422 {
		t.Fatal("cursor", r.status)
	}
}
