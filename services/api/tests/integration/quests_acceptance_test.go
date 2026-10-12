//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"sync"
	"testing"
	"time"
)

func TestQuestConcurrentEditorsAndCompletionRollback(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "concurrent-quests@example.test")
	q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput("Исходная")))
	results := make(chan authResponse, 2)
	var work sync.WaitGroup
	for _, title := range []string{"Первый редактор", "Второй редактор"} {
		work.Add(1)
		go func() {
			defer work.Done()
			in := q.Input
			in.Title = title
			results <- f.privateRequest(t, client, "PATCH", "/quests/"+q.ID, version(q), in)
		}()
	}
	work.Wait()
	close(results)
	ok, conflict := 0, 0
	for r := range results {
		if r.status == 200 {
			ok++
		}
		if r.status == 409 {
			conflict++
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal("concurrent edit overwritten", ok, conflict)
	}
	q = readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+q.ID, "", nil))
	if q.Version != 2 {
		t.Fatal("wrong concurrent version")
	}
	if _, err := f.pool.Exec(f.ctx, `CREATE FUNCTION reject_quest_completion() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='complete' THEN RAISE EXCEPTION 'test journal unavailable'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `CREATE TRIGGER reject_quest_completion BEFORE INSERT ON task_events FOR EACH ROW EXECUTE FUNCTION reject_quest_completion()`); err != nil {
		t.Fatal(err)
	}
	if r := f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}, "failed-completion-key"); r.status != 500 {
		t.Fatal("journal failure accepted")
	}
	after := readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+q.ID, "", nil))
	if after.Version != q.Version || after.Status != q.Status || after.CompletedAt != nil {
		t.Fatal("completion not rolled back")
	}
	var records int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM idempotency_records WHERE idempotency_key='failed-completion-key'`).Scan(&records); err != nil || records != 0 {
		t.Fatal("failed operation cached")
	}
	if _, err := f.pool.Exec(f.ctx, `DROP TRIGGER reject_quest_completion ON task_events`); err != nil {
		t.Fatal(err)
	}
	results = make(chan authResponse, 8)
	for range 8 {
		work.Add(1)
		go func() {
			defer work.Done()
			results <- f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}, "parallel-completion-key")
		}()
	}
	work.Wait()
	close(results)
	for r := range results {
		completed := readQuest(t, r)
		if completed.Version != q.Version+1 || completed.Status != "completed" {
			t.Fatal("parallel completion differs")
		}
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM task_events WHERE quest_id=$1 AND kind='complete'`, q.ID).Scan(&records); err != nil || records != 1 {
		t.Fatal("parallel completion duplicated")
	}
}
func TestQuestDependencyChainRollup(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "rollup-chain@example.test")
	create := func(title string, parent *string) quests.Quest {
		in := questInput(title)
		in.ParentID = parent
		return readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", in))
	}
	load := func(q quests.Quest) quests.Quest {
		return readQuest(t, f.privateRequest(t, client, "GET", "/quests/"+q.ID, "", nil))
	}
	a := create("A", nil)
	leafA := create("Лист A", &a.ID)
	b := create("B", nil)
	leafB := create("Лист B", &b.ID)
	c := create("C", nil)
	leafC := create("Лист C", &c.ID)
	b = readQuest(t, f.privateRequest(t, client, "PUT", "/quests/"+b.ID+"/dependencies", version(b), quests.DependenciesInput{IDs: []string{a.ID}}))
	c = readQuest(t, f.privateRequest(t, client, "PUT", "/quests/"+c.ID+"/dependencies", version(c), quests.DependenciesInput{IDs: []string{b.ID}}))
	for _, q := range []quests.Quest{leafB, leafC} {
		readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	}
	if load(b).Status == "completed" || load(c).Status == "completed" {
		t.Fatal("blocked containers completed")
	}
	leafA = readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+leafA.ID+"/complete", version(leafA), quests.Transition{}))
	for _, q := range []quests.Quest{a, b, c} {
		if load(q).Status != "completed" {
			t.Fatal("dependency chain not completed", q.Title)
		}
	}
	readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+leafA.ID+"/reopen", version(leafA), quests.Transition{}))
	for _, q := range []quests.Quest{a, b, c} {
		if load(q).Status != "todo" {
			t.Fatal("dependency chain not reopened", q.Title)
		}
	}
	b = load(b)
	b = readQuest(t, f.privateRequest(t, client, "PUT", "/quests/"+b.ID+"/dependencies", version(b), quests.DependenciesInput{IDs: []string{}, Reason: "Больше не блокирует"}))
	if b.Status != "completed" || load(c).Status != "completed" {
		t.Fatal("removing dependency did not roll up containers")
	}
	if load(leafB).Status != "completed" || load(leafC).Status != "completed" || load(leafA).Status != "todo" {
		t.Fatal("rollup changed leaves")
	}
}

func TestAttachmentAccountQuota(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "account-quota@example.test")
	q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", questInput("Новый файл")))
	input, err := json.Marshal(questInput("Quota fixture"))
	if err != nil {
		t.Fatal(err)
	}
	// Metadata in this isolated schema fills the account quota without allocating
	// 250 MB of test files; the actual upload still uses the HTTP boundary.
	_, err = f.pool.Exec(f.ctx, `WITH created AS (
 INSERT INTO quests(owner_id,payload,status,created_at,updated_at)
 SELECT $1,$2,'todo',$3,$3 FROM generate_series(1,25) RETURNING id)
 INSERT INTO attachments(owner_id,quest_id,storage_key,name,mime,size,checksum,created_at)
 SELECT $1,id,lpad((row_number() OVER())::text,64,'0'),'quota.txt','text/plain; charset=utf-8',10485760,repeat('0',64),$3 FROM created`, account.ID, input, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r := f.uploadFile(t, client, q, "beyond.txt", []byte("over quota")); r.status != 422 {
		t.Fatal("account quota ignored", r.status)
	}
}
func TestQuestDateTypesFiltersAndLabelOwnership(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, _ := f.signedIn(t, "date-quests@example.test")
	other, _ := f.signedIn(t, "date-labels@example.test")
	area := readLabel(t, f.privateRequest(t, other, "POST", "/areas", "", map[string]any{"name": "Чужое направление", "color": "#6478c8", "icon": "book", "archived": false}))
	tag := readLabel(t, f.privateRequest(t, other, "POST", "/tags", "", map[string]any{"name": "Чужой тег", "color": "#6478c8", "icon": "tag", "archived": false}))
	in := questInput("Календарная дата")
	in.AreaID = &area.ID
	if r := f.privateRequest(t, client, "POST", "/quests", "", in); r.status != 422 {
		t.Fatal("foreign area")
	}
	in.AreaID = nil
	in.Tags = []string{tag.ID}
	if r := f.privateRequest(t, client, "POST", "/quests", "", in); r.status != 422 {
		t.Fatal("foreign tag")
	}
	in.Tags = []string{}
	date := "2026-03-08"
	in.PlannedDate = &date
	deadline, err := time.Parse(time.RFC3339, "2026-03-08T01:30:00-05:00")
	if err != nil {
		t.Fatal(err)
	}
	in.Deadline = &deadline
	q := readQuest(t, f.privateRequest(t, client, "POST", "/quests", "", in))
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	for _, zone := range []string{"UTC", "Asia/Almaty", "America/New_York"} {
		if _, err := tx.Exec(f.ctx, `SELECT set_config('TimeZone',$1,true)`, zone); err != nil {
			t.Fatal(err)
		}
		var stored, dayType, deadlineType string
		var moment time.Time
		if err := tx.QueryRow(f.ctx, `SELECT planned_date::text,pg_typeof(planned_date)::text,deadline,pg_typeof(deadline)::text FROM quests WHERE id=$1`, q.ID).Scan(&stored, &dayType, &moment, &deadlineType); err != nil {
			t.Fatal(err)
		}
		if stored != date || dayType != "date" || deadlineType != "timestamp with time zone" || !moment.Equal(deadline) {
			t.Fatal("timezone changed the day or moment", zone)
		}
	}
	r := f.privateRequest(t, client, "GET", "/quests?planned_from=2026-03-08&planned_to=2026-03-08&overdue=true", "", nil)
	var page struct {
		Data quests.Page `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &page) != nil || page.Data.Total != 1 {
		t.Fatal("date/overdue filter")
	}
	readQuest(t, f.privateRequest(t, client, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	r = f.privateRequest(t, client, "GET", "/quests?overdue=true", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &page) != nil || page.Data.Total != 0 {
		t.Fatal("terminal overdue")
	}
	if r := f.privateRequest(t, client, "GET", "/quests?planned_from=0000-01-01", "", nil); r.status != 422 {
		t.Fatal("invalid year")
	}
}
