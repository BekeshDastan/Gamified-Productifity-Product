//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/character"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func readCharacter(t *testing.T, r authResponse) character.Profile {
	t.Helper()
	var out struct {
		Data character.Profile `json:"data"`
	}
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatalf("character %d %s", r.status, r.body)
	}
	return out.Data
}
func characterVersion(p character.Profile) string { return fmt.Sprintf(`"%d"`, p.Version) }
func TestCharacterInventoryOwnershipSlotsAndPersistence(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "character-a@example.test")
	b, _ := f.signedIn(t, "character-b@example.test")
	p := readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
	owned := 0
	for _, i := range p.Inventory {
		if i.Owned {
			owned++
		}
	}
	if owned != 6 || len(p.Equipment) != 6 || len(p.Inventory) != 20 {
		t.Fatal(p)
	}
	input := map[string]string{"slot": "clothes", "item_id": "clothes-1"}
	if r := f.privateRequest(t, a, "PATCH", "/character/equipment", characterVersion(p), input); r.status != 409 {
		t.Fatal("unearned", r.status)
	}
	input["item_id"] = "head-0"
	if r := f.privateRequest(t, a, "PATCH", "/character/equipment", characterVersion(p), input); r.status != 422 {
		t.Fatal("slot", r.status)
	}
	qInput := questInput("Открыть одежду")
	qInput.Difficulty = 5
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", qInput))
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{"reason": "Готово"}))
	p = readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
	input["item_id"] = "clothes-1"
	key := "character-equip-once-0001"
	v := characterVersion(p)
	p = readCharacter(t, f.privateRequest(t, a, "PATCH", "/character/equipment", v, input, key))
	again := readCharacter(t, f.privateRequest(t, a, "PATCH", "/character/equipment", v, input, key))
	if p.Equipment["clothes"] != "clothes-1" || p.Version != again.Version {
		t.Fatal(p, again)
	}
	other := readCharacter(t, f.privateRequest(t, b, "GET", "/character", "", nil))
	if r := f.privateRequest(t, b, "PATCH", "/character/equipment", characterVersion(other), input); r.status != 409 {
		t.Fatal("borrowed another owner's inventory", r.status)
	}
	next := authClient(t)
	f.loginAs(t, next, "character-a@example.test")
	fresh := readCharacter(t, f.privateRequest(t, next, "GET", "/character", "", nil))
	if fresh.Equipment["clothes"] != "clothes-1" {
		t.Fatal("session persistence")
	}
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/reopen", version(q), map[string]string{"reason": "Исправление"}))
	fresh = readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
	if fresh.Equipment["clothes"] != "clothes-0" {
		t.Fatal("revoked item remains equipped", fresh)
	}
	if r := f.privateRequest(t, a, "PATCH", "/character/equipment", characterVersion(p), input); r.status != 409 {
		t.Fatal("stale version", r.status)
	}
}

func TestCharacterAreaXPUsesOriginalQualifiedLedgerOnly(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, owner := f.signedIn(t, "area-ledger@example.test")
	areaA, areaB := testkit.NewUser().ID, testkit.NewUser().ID
	for _, id := range []string{areaA, areaB} {
		if _, e := f.pool.Exec(t.Context(), "INSERT INTO life_areas(id,owner_id,name,color,icon,created_at,updated_at) VALUES($1,$2,$4,'#23575b','book',$3,$3)", id, owner.ID, f.clock.Now(), id); e != nil {
			t.Fatal(e)
		}
	}
	in := questInput("Работа в направлении")
	in.AreaID = &areaA
	in.Difficulty = 3
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", in))
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{"reason": "Готово"}))
	assert := func(xp int64) {
		p := readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
		var sum int64
		for _, area := range p.Areas {
			sum += area.XP
			if area.ID == areaB && area.XP != 0 {
				t.Fatal("changed reward snapshot")
			}
		}
		var total int64
		if e := f.pool.QueryRow(t.Context(), "SELECT total_xp FROM user_progress WHERE owner_id=$1", owner.ID).Scan(&total); e != nil || total != xp || sum != xp {
			t.Fatal("global XP double counted", total, sum, e)
		}
	}
	assert(25)
	in = q.Input
	in.AreaID = &areaB
	q = readQuest(t, f.privateRequest(t, a, "PATCH", "/quests/"+q.ID, version(q), in))
	assert(25)
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/reopen", version(q), map[string]string{"reason": "Исправление"}))
	assert(0)
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{"reason": "Восстановление"}))
	assert(25)
}

func TestCharacterAchievementEventsRepeatsCorrectionsAndMassRecompute(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, owner := f.signedIn(t, "achievement-a@example.test")
	b, _ := f.signedIn(t, "achievement-b@example.test")
	pService := progression.NewService(f.pool, f.clock)
	svc := character.NewService(f.pool, f.clock, pService)
	handler := svc.Handlers()["character.changed"]
	job := jobs.Job{OwnerID: &owner.ID, Payload: []byte("{}")}
	run := func() {
		e := pService.WithinOwner(t.Context(), owner.ID, func(tx pgx.Tx) error { return handler(t.Context(), tx, job) })
		if e != nil {
			t.Fatal(e)
		}
	}
	in := questInput("Первый шаг")
	in.Difficulty = 3
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", in))
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{"reason": "Готово"}))
	run()
	run()
	var first time.Time
	var active bool
	if e := f.pool.QueryRow(t.Context(), "SELECT active,awarded_at FROM user_achievements WHERE owner_id=$1 AND definition_id='task-1'", owner.ID).Scan(&active, &first); e != nil || !active {
		t.Fatal(active, e)
	}
	p := readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
	p = readCharacter(t, f.privateRequest(t, a, "PATCH", "/character/equipment", characterVersion(p), map[string]string{"slot": "accessory", "item_id": "accessory-1"}))
	other := readCharacter(t, f.privateRequest(t, b, "GET", "/character", "", nil))
	if other.Equipment["accessory"] == "accessory-1" {
		t.Fatal("foreign grant")
	}
	f.clock.Advance(time.Minute)
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/reopen", version(q), map[string]string{"reason": "Ошибка в отметке"}))
	run()
	run()
	p = readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
	if p.Equipment["accessory"] != "accessory-0" {
		t.Fatal("corrected unlock still equipped")
	}
	var after time.Time
	if e := f.pool.QueryRow(t.Context(), "SELECT active,awarded_at FROM user_achievements WHERE owner_id=$1 AND definition_id='task-1'", owner.ID).Scan(&active, &after); e != nil || active || !first.Equal(after) {
		t.Fatal("lost correction history", active, after, e)
	}
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{"reason": "Восстановление"}))
	run()
	for range 2 {
		if n, e := svc.RecomputeAll(t.Context()); e != nil || n != 2 {
			t.Fatal("batch", n, e)
		}
	}
	var grants, changes, xp int64
	if e := f.pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM user_achievements WHERE owner_id=$1 AND definition_id='task-1'),(SELECT count(*) FROM achievement_changes WHERE owner_id=$1 AND definition_id='task-1'),total_xp FROM user_progress WHERE owner_id=$1", owner.ID).Scan(&grants, &changes, &xp); e != nil || grants != 1 || changes != 3 || xp != 25 {
		t.Fatal("duplicate grant or XP", grants, changes, xp, e)
	}
	if r := f.privateRequest(t, b, "GET", "/achievements", "", nil); r.status != 200 {
		t.Fatal(r.status)
	}
	var result struct{ Data []character.AchievementStatus }
	r := f.privateRequest(t, a, "GET", "/achievements", "", nil)
	if json.Unmarshal(r.body, &result) != nil || len(result.Data) != 30 {
		t.Fatal("catalog response", string(r.body))
	}
	for _, d := range result.Data {
		if (d.Metric == character.FocusSessions || d.Metric == character.CompletedSeasons || d.Metric == character.CoopChallenges) && (d.Active || d.Available) {
			t.Fatal("invented unavailable evidence", d)
		}
	}
}

func TestCharacterTitlesAndRecordsFollowCorrectedEvidence(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "record-a@example.test")
	b, _ := f.signedIn(t, "record-b@example.test")
	in := habitInput()
	in.StartsOn = "2026-10-05"
	in.Schedule.Weekdays = []int{1, 2, 3, 4, 5, 6, 7}
	h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", in))
	periods := append([]habits.Period{}, h.Periods...)
	for _, p := range periods {
		if !p.End.After(f.clock.Now()) {
			h = readHabit(t, f.privateRequest(t, a, "POST", "/habits/"+h.ID+"/logs", habitVersion(h), habits.LogInput{PeriodID: p.ID, Amount: 20, OccurredAt: p.Start.Add(time.Hour)}))
		}
	}
	p := readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
	record := func(p character.Profile, id string) *int64 {
		for _, r := range p.Records {
			if r.ID == id {
				if id != "longest-focus" && len(r.Sources) == 0 {
					t.Fatal("record without evidence", r)
				}
				return r.Value
			}
		}
		return nil
	}
	if v := record(p, "best-streak"); v == nil || *v != 7 {
		t.Fatal("best", p.Records)
	}
	if v := record(p, "weekly-regularity"); v == nil || *v != 100 {
		t.Fatal("weekly", p.Records)
	}
	if v := record(p, "longest-focus"); v != nil {
		t.Fatal("invented focus")
	}
	if r := f.privateRequest(t, a, "PATCH", "/character/title", characterVersion(p), map[string]string{"title_id": "level:50"}); r.status != 409 {
		t.Fatal("unearned level title", r.status)
	}
	p = readCharacter(t, f.privateRequest(t, a, "PATCH", "/character/title", characterVersion(p), map[string]string{"title_id": "achievement:streak-7"}))
	other := readCharacter(t, f.privateRequest(t, b, "GET", "/character", "", nil))
	if r := f.privateRequest(t, b, "PATCH", "/character/title", characterVersion(other), map[string]string{"title_id": p.TitleID}); r.status != 409 {
		t.Fatal("foreign achievement title", r.status)
	}
	logs := readHabitLogs(t, f.privateRequest(t, a, "GET", "/habits/"+h.ID+"/logs", "", nil))
	logID := ""
	for _, log := range logs {
		if log.OccurredAt.UTC().Format(time.DateOnly) == "2026-10-11" {
			logID = log.ID
		}
	}
	if logID == "" {
		t.Fatal("missing source log")
	}
	h = readHabit(t, f.privateRequest(t, a, "POST", "/habits/"+h.ID+"/logs/"+logID+"/correct", habitVersion(h), habits.CorrectionInput{Amount: 0, Reason: "Отметка была ошибочной"}))
	p = readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
	if p.TitleID != "level:auto" {
		t.Fatal("unearned corrected title", p.TitleID)
	}
	if v := record(p, "best-streak"); v == nil || *v != 6 {
		t.Fatal("stale best", p.Records)
	}
	if v := record(p, "weekly-regularity"); v == nil || *v != 85 {
		t.Fatal("stale weekly", p.Records)
	}
}
