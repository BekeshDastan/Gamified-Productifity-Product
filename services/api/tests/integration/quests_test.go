//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"testing"
	"time"
)

func questInput(title string) quests.Input {
	return quests.Input{Title: title, Priority: "normal", Difficulty: 2, Tags: []string{}, Status: "todo", Required: true}
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
func TestQuestFiltersAndViews(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "filters@example.test")
	other, _ := f.signedIn(t, "filters-other@example.test")
	for _, title := range []string{"Alpha", "Beta", "Alphabet"} {
		in := questInput(title)
		in.Priority = "high"
		readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", in))
	}
	r := f.privateRequest(t, client, "GET", "/quests?search=Alp&priority=high&sort=title", "", nil)
	var body struct {
		Data quests.Page `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &body) != nil || body.Data.Total != 2 || body.Data.Items[0].Title != "Alpha" {
		t.Fatal("filtered order", r.status, string(r.body))
	}
	r = f.privateRequest(t, client, "POST", "/quests/views", "", quests.ViewInput{Name: "Важное", Filters: quests.Filters{Search: "Alp", Priority: "high", Sort: "title"}})
	var view struct {
		Data quests.View `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &view) != nil {
		t.Fatal("view create")
	}
	if r = f.privateRequest(t, other, "DELETE", "/quests/views/"+view.Data.ID, `"1"`, nil); r.status != 404 {
		t.Fatal("foreign view removed")
	}
	if r = f.privateRequest(t, client, "GET", "/quests/views", "", nil); r.status != 200 || !bytes.Contains(r.body, []byte("Важное")) {
		t.Fatal("view lost")
	}
	if r = f.privateRequest(t, client, "GET", "/quests?sort=arbitrary", "", nil); r.status != 422 {
		t.Fatal("invalid order")
	}
}
func TestQuestLifecycleHistoryAndIdempotency(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "lifecycle@example.test")
	in := questInput("Статусы")
	deadline := f.clock.Now().Add(time.Hour)
	in.Deadline = &deadline
	q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", in))
	action := func(name string, body quests.Transition) {
		t.Helper()
		q = readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/"+name, version(q), body))
	}
	action("start", quests.Transition{})
	date := "2026-10-15"
	action("postpone", quests.Transition{Reason: "Смена плана", PlannedDate: &date})
	if q.Deadline == nil || !q.Deadline.Equal(deadline) || q.PlannedDate == nil || *q.PlannedDate != date {
		t.Fatal("postpone moved deadline")
	}
	action("ready", quests.Transition{})
	action("skip", quests.Transition{Reason: "Больше не нужно"})
	action("reopen", quests.Transition{})
	action("fail", quests.Transition{Reason: "Не успел"})
	action("reopen", quests.Transition{})
	key := "complete-once-test-key"
	oldVersion := version(q)
	first := readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/complete", oldVersion, quests.Transition{}, key))
	again := readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/complete", oldVersion, quests.Transition{}, key))
	if first.Version != again.Version || first.CompletedAt == nil || first.Status != "completed" {
		t.Fatal("duplicate completion")
	}
	if r := f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/complete", oldVersion, quests.Transition{Reason: "changed"}, key); r.status != 409 {
		t.Fatal("key body mismatch")
	}
	q = first
	if r := f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/start", version(q), quests.Transition{}); r.status != 409 {
		t.Fatal("terminal transition")
	}
	action("reopen", quests.Transition{})
	action("postpone", quests.Transition{Reason: "Вернуть в идеи", ReturnToBacklog: true})
	if q.Status != "backlog" || q.PlannedDate != nil {
		t.Fatal("backlog return")
	}
	if r := f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/postpone", version(q), quests.Transition{Reason: "x", PlannedDate: &date}); r.status != 409 {
		t.Fatal("backlog postponed")
	}
	r := f.privateRequest(t, client, "GET", "/quests/"+q.ID+"/history", "", nil)
	if r.status != 200 || !bytes.Contains(r.body, []byte("Смена плана")) || !bytes.Contains(r.body, []byte("Больше не нужно")) {
		t.Fatal("missing history")
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM task_events WHERE quest_id=$1 AND kind='complete'`, q.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("completion event duplicated", count, err)
	}
}
func TestQuestHierarchyAndDependencies(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "tree@example.test")
	other, _ := f.signedIn(t, "tree-other@example.test")
	root := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput("Проект")))
	in := questInput("Глава")
	in.ParentID = &root.ID
	child := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", in))
	in = questInput("Страница")
	in.ParentID = &child.ID
	leaf := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", in))
	in = questInput("Глубже")
	in.ParentID = &leaf.ID
	if r := f.privateRequest(t, client, "POST", "/quests", "", in); r.status != 422 {
		t.Fatal("depth accepted")
	}
	in = root.Input
	in.ParentID = &leaf.ID
	if r := f.privateRequest(t, client, "PATCH", "/quests/"+root.ID, version(root), in); r.status != 422 {
		t.Fatal("tree cycle")
	}
	if r := f.privateRequest(t, client, "POST", "/quests/"+root.ID+"/complete", version(root), quests.Transition{}); r.status != 409 {
		t.Fatal("parent completed children")
	}
	foreign := readQuest(t, f.privateRequest(t, other, "POST", "/quests", "", questInput("Чужая")))
	in = questInput("Чужой родитель")
	in.ParentID = &foreign.ID
	if r := f.privateRequest(t, client, "POST", "/quests", "", in); r.status != 422 {
		t.Fatal("foreign parent")
	}
	leaf = readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+leaf.ID+"/complete", version(leaf), quests.Transition{}))
	root = readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+root.ID, "", nil))
	child = readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+child.ID, "", nil))
	if root.Status != "completed" || child.Status != "completed" {
		t.Fatal("container rollup")
	}
	readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+leaf.ID+"/reopen", version(leaf), quests.Transition{}))
	root = readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+root.ID, "", nil))
	if root.Status != "todo" {
		t.Fatal("rollup reopen")
	}
	a := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput("A")))
	b := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput("B")))
	b = readQuest(t, f.privateRequest(t, client, "PUT", "/quests/"+b.ID+"/dependencies", version(b), quests.DependenciesInput{IDs: []string{a.ID}}))
	if r := f.privateRequest(t, client, "PUT", "/quests/"+a.ID+"/dependencies", version(a), quests.DependenciesInput{IDs: []string{b.ID}}); r.status != 422 {
		t.Fatal("dependency cycle")
	}
	if r := f.privateRequest(t, client, "PUT", "/quests/"+a.ID+"/dependencies", version(a), quests.DependenciesInput{IDs: []string{foreign.ID}}); r.status != 422 {
		t.Fatal("foreign dependency")
	}
	if r := f.privateRequest(t, client, "POST", "/quests/"+b.ID+"/start", version(b), quests.Transition{}); r.status != 409 {
		t.Fatal("blocked task started")
	}
	a = readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+a.ID+"/skip", version(a), quests.Transition{Reason: "Отказ"}))
	if r := f.privateRequest(t, client, "POST", "/quests/"+b.ID+"/complete", version(b), quests.Transition{}); r.status != 409 {
		t.Fatal("skipped dependency unblocked")
	}
	if r := f.privateRequest(t, client, "PUT", "/quests/"+b.ID+"/dependencies", version(b), quests.DependenciesInput{IDs: []string{}}); r.status != 422 {
		t.Fatal("silent unblock")
	}
	b = readQuest(t, f.privateRequest(t, client, "PUT", "/quests/"+b.ID+"/dependencies", version(b), quests.DependenciesInput{IDs: []string{}, Reason: "Снята зависимость"}))
	readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+b.ID+"/start", version(b), quests.Transition{}))
}
