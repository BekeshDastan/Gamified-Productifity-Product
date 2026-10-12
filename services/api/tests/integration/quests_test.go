//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"testing"
)

func questInput(title string) quests.Input {
	return quests.Input{Title: title, Priority: "normal", Difficulty: 2, Tags: []string{}, Status: "todo"}
}
func readQuest(t *testing.T, r authResponse) quests.Quest {
	t.Helper()
	var body struct {
		Data quests.Quest `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &body) != nil || body.Data.ID == "" {
		t.Fatalf("quest response: %d %s", r.status, r.body)
	}
	return body.Data
}
func version(q quests.Quest) string { return fmt.Sprintf(`"%d"`, q.Version) }
func TestQuestCRUDOwnerPaginationAndStarter(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "quests@example.test")
	other, _ := f.signedIn(t, "quests-other@example.test")
	input := questInput("Прочитать главу")
	q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", input))
	input.Title = "Прочитать две главы"
	q = readQuest(t, f.privateRequest(t, client, "PATCH", "/quests/"+q.ID, version(q), input))
	if q.Version != 2 || q.Title != input.Title {
		t.Fatal("edit lost")
	}
	if r := f.privateRequest(t, client, "PATCH", "/quests/"+q.ID, `"1"`, input); r.status != 409 {
		t.Fatal("stale overwrite")
	}
	if r := f.privateRequest(t, other, "GET", "/quests/"+q.ID, "", nil); r.status != 404 {
		t.Fatal("foreign read")
	}
	if r := f.privateRequest(t, other, "PATCH", "/quests/"+q.ID, version(q), input); r.status != 404 {
		t.Fatal("foreign edit")
	}
	input.Title = ""
	if r := f.privateRequest(t, client, "POST", "/quests", "", input); r.status != 422 {
		t.Fatal("empty title")
	}
	if r := f.privateRequest(t, client, "POST", "/quests", "", map[string]any{"owner_id": account.ID}); r.status != 422 {
		t.Fatal("owner accepted")
	}
	_, err := f.pool.Exec(f.ctx, `INSERT INTO onboarding_starters(owner_id,kind,title,created_at,updated_at) VALUES($1,'task','Первая задача',$2,$2)`, account.ID, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	var source string
	for range 2 {
		r := f.privateRequest(t, client, "GET", "/quests?page_size=1&page=2", "", nil)
		var body struct {
			Data quests.Page `json:"data"`
		}
		if r.status != 200 || json.Unmarshal(r.body, &body) != nil || body.Data.Total != 2 || len(body.Data.Items) != 1 {
			t.Fatal("pagination/promotion", r.status)
		}
		var promoted string
		if err := f.pool.QueryRow(f.ctx, `SELECT promoted_entity_id::text FROM onboarding_starters WHERE owner_id=$1 AND kind='task'`, account.ID).Scan(&promoted); err != nil {
			t.Fatal(err)
		}
		if source != "" && source != promoted {
			t.Fatal("draft duplicated")
		}
		source = promoted
	}
}
