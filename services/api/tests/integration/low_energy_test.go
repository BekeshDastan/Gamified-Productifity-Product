//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/seasons"
	"sync"
	"testing"
	"time"
)

func readLowEnergy(t *testing.T, r authResponse) seasons.LowEnergyPreview {
	t.Helper()
	var v struct{ Data seasons.LowEnergyPreview }
	if r.status != 200 || json.Unmarshal(r.body, &v) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return v.Data
}
func TestLowEnergyProtectsDeadlinesEventsAndOriginalScore(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "low-energy@example.test")
	b, _ := f.signedIn(t, "low-foreign@example.test")
	date := "2026-10-12"
	task := questInput("Срочная задача")
	task.EstimatedMinutes = 60
	deadline := time.Date(2026, 10, 12, 20, 0, 0, 0, time.UTC)
	task.Deadline = &deadline
	task.PlannedDate = &date
	urgent := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", task))
	task.Title = "Небольшая гибкая задача"
	task.Deadline = nil
	task.EstimatedMinutes = 10
	small := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", task))
	task.Title = "Большая гибкая задача"
	task.EstimatedMinutes = 90
	large := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", task))
	input := planning.Input{AvailableMinutes: 180, QuestIDs: []string{urgent.ID, small.ID, large.ID}, TopIDs: []string{small.ID}, Windows: []planning.Window{{Start: "09:00", End: "12:00"}}}
	path := "/plans/day/" + date
	p := readPlan(t, f.privateRequest(t, a, "PUT", path, `"0"`, input))
	p = readPlan(t, f.privateRequest(t, a, "POST", path+"/confirm", planVersion(p), struct{}{}))
	event := readEvent(t, f.privateRequest(t, a, "POST", "/calendar/events", "", calendar.Input{Title: "Важное событие", AllDay: true, Start: date, End: "2026-10-13", Timezone: "UTC"}))
	v := readLowEnergy(t, f.privateRequest(t, a, "POST", "/recovery/low-energy/preview", "", seasons.LowEnergyInput{Date: date, MinimumMinutes: 10}))
	if len(v.Commitments) != 1 || v.Commitments[0].ID != event.ID || !v.Candidates[0].Required || !v.Candidates[1].Proposed || v.Candidates[2].Proposed {
		t.Fatal(v)
	}
	unchanged := readPlan(t, f.privateRequest(t, a, "GET", path, "", nil))
	if unchanged.Version != p.Version || len(unchanged.QuestIDs) != 3 {
		t.Fatal("preview changed plan")
	}
	apply := seasons.LowEnergyApply{Confirm: true, Reason: "Сегодня меньше энергии", OptionalIDs: []string{small.ID}}
	applyPath := "/recovery/low-energy/" + v.ID + "/apply"
	tooLarge := apply
	tooLarge.OptionalIDs = []string{large.ID}
	if r := f.privateRequest(t, a, "POST", applyPath, "", tooLarge); r.status != 422 {
		t.Fatal("budget ignored", r.status)
	}
	protected := apply
	protected.OptionalIDs = []string{urgent.ID}
	if r := f.privateRequest(t, a, "POST", applyPath, "", protected); r.status != 422 {
		t.Fatal("protected task treated as optional", r.status)
	}
	if r := f.privateRequest(t, b, "POST", applyPath, "", apply); r.status != 404 {
		t.Fatal(r.status, string(r.body))
	}
	done := readLowEnergy(t, f.privateRequest(t, a, "POST", applyPath, "", apply))
	if done.Result == nil || len(done.Result.QuestIDs) != 2 || len(done.Result.Snapshot.Items) != 3 || done.Result.Progress.Committed != 3 || done.Result.AvailableMinutes != 180 {
		t.Fatal("original commitment rewritten", done)
	}
	var eventVersion int64
	var cancelled bool
	if e := f.pool.QueryRow(f.ctx, "SELECT version,cancelled FROM calendar_events WHERE owner_id=$1 AND id=$2", who.ID, event.ID).Scan(&eventVersion, &cancelled); e != nil {
		t.Fatal(e)
	}
	if eventVersion != event.Version || cancelled {
		t.Fatal("fixed event removed")
	}
	originalQuest := readQuest(t, f.privateRequest(t, a, "GET", "/quests/"+large.ID, "", nil))
	if originalQuest.Version != large.Version || originalQuest.PlannedDate == nil || *originalQuest.PlannedDate != date {
		t.Fatal("task silently rescheduled")
	}
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "low-energy@example.test")
	replay := readLowEnergy(t, f.privateRequest(t, a, "POST", applyPath, "", apply))
	if replay.Result.Version != done.Result.Version {
		t.Fatal("duplicate apply")
	}
	apply.Reason = "Другая причина"
	if r := f.privateRequest(t, a, "POST", applyPath, "", apply); r.status != 409 {
		t.Fatal(r.status)
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM reward_ledger WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 0 {
		t.Fatal("reduction grants or deducts XP", n, e)
	}
}
func TestLowEnergyStaleBusyDayExpiryAndUnknownEstimate(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "low-stale@example.test")
	date := "2026-10-12"
	task := questInput("Без оценки")
	task.EstimatedMinutes = 0
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", task))
	p := readPlan(t, f.privateRequest(t, a, "PUT", "/plans/day/"+date, `"0"`, planning.Input{AvailableMinutes: 60, QuestIDs: []string{q.ID}, Windows: []planning.Window{{Start: "09:00", End: "10:00"}}}))
	_ = p
	preview := func() seasons.LowEnergyPreview {
		return readLowEnergy(t, f.privateRequest(t, a, "POST", "/recovery/low-energy/preview", "", seasons.LowEnergyInput{Date: date, MinimumMinutes: 0}))
	}
	v := preview()
	in := seasons.LowEnergyApply{Confirm: true, Reason: "Маленький шаг", OptionalIDs: []string{q.ID}}
	path := func(v seasons.LowEnergyPreview) string { return "/recovery/low-energy/" + v.ID + "/apply" }
	if v.Candidates[0].Proposed || v.Candidates[0].Reason != "unknown_estimate" {
		t.Fatal(v)
	}
	for _, bad := range []seasons.LowEnergyApply{
		{Confirm: false, Reason: "Нет согласия", OptionalIDs: []string{}},
		{Confirm: true, Reason: "", OptionalIDs: []string{}},
		{Confirm: true, Reason: "bad\x00reason", OptionalIDs: []string{}},
		{Confirm: true, Reason: "Повтор выбора", OptionalIDs: []string{q.ID, q.ID}, ConfirmUnknown: true},
		{Confirm: true, Reason: "Чужой выбор", OptionalIDs: []string{"00000000-0000-4000-8000-000000000001"}},
	} {
		if r := f.privateRequest(t, a, "POST", path(v), "", bad); r.status != 422 {
			t.Fatal("invalid choice accepted", r.status, string(r.body))
		}
	}
	if r := f.privateRequest(t, a, "POST", path(v), "", in); r.status != 422 {
		t.Fatal(r.status, string(r.body))
	}
	readEvent(t, f.privateRequest(t, a, "POST", "/calendar/events", "", calendar.Input{Title: "Новая встреча", Start: date + "T09:00:00Z", End: date + "T10:00:00Z", Timezone: "UTC"}))
	in.ConfirmUnknown = true
	if r := f.privateRequest(t, a, "POST", path(v), "", in); r.status != 409 {
		t.Fatal("new busy event undetected", r.status, string(r.body))
	}
	v = preview()
	if len(v.Commitments) != 1 {
		t.Fatal(v)
	}
	f.clock.Advance(10 * time.Minute)
	if r := f.privateRequest(t, a, "POST", path(v), "", in); r.status != 409 {
		t.Fatal("expiry", r.status)
	}
	v = preview()
	readLowEnergy(t, f.privateRequest(t, a, "POST", path(v), "", in))
	if r := f.privateRequest(t, a, "POST", "/recovery/low-energy/preview", "", seasons.LowEnergyInput{Date: "2026-10-11"}); r.status == 200 {
		t.Fatal("past plan", r.status)
	}
}
func TestLowEnergyFreezeTimezoneRollbackAndConcurrentApply(t *testing.T) {
	limits := auth.DefaultLimits()
	limits.UserRequests = 600
	f := newAuthFixture(t, limits)
	a, who := f.signedIn(t, "low-atomic@example.test")
	date := "2026-10-12"
	makeTask := func(title string, minutes int) quests.Quest {
		in := questInput(title)
		in.EstimatedMinutes = minutes
		in.PlannedDate = &date
		return readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", in))
	}
	fixed := makeTask("С фиксированным блоком", 60)
	optional := makeTask("Гибкая", 10)
	done := makeTask("Уже сделано", 20)
	in := planning.Input{AvailableMinutes: 60, QuestIDs: []string{fixed.ID, optional.ID, done.ID}, TopIDs: []string{optional.ID}, Windows: []planning.Window{{Start: "09:00", End: "10:00"}}}
	p := readPlan(t, f.privateRequest(t, a, "PUT", "/plans/day/"+date, `"0"`, in))
	p = readPlan(t, f.privateRequest(t, a, "POST", "/plans/day/"+date+"/confirm", planVersion(p), struct{}{}))
	readEvent(t, f.privateRequest(t, a, "POST", "/plans/day/"+date+"/blocks", planVersion(p), planning.Block{Move: planning.Move{QuestID: fixed.ID, QuestVersion: fixed.Version, Reason: "Выделено время"}, Start: "09:00", End: "10:00"}))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+done.ID+"/complete", version(done), quests.Transition{}))
	preview := func() seasons.LowEnergyPreview {
		return readLowEnergy(t, f.privateRequest(t, a, "POST", "/recovery/low-energy/preview", "", seasons.LowEnergyInput{Date: date, MinimumMinutes: 10}))
	}
	v := preview()
	if !v.Candidates[0].Required || v.Candidates[0].Reason != "fixed_commitment" || v.Candidates[2].Eligible {
		t.Fatal(v)
	}
	if _, e := f.pool.Exec(f.ctx, "UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}',to_jsonb('Asia/Almaty'::text)),version=version+1 WHERE owner_id=$1", who.ID); e != nil {
		t.Fatal(e)
	}
	apply := seasons.LowEnergyApply{Confirm: true, Reason: "Нужен щадящий план", OptionalIDs: []string{optional.ID}}
	path := func(id string) string { return "/recovery/low-energy/" + id + "/apply" }
	if r := f.privateRequest(t, a, "POST", path(v.ID), "", apply); r.status != 409 {
		t.Fatal("timezone update not detected", r.status)
	}
	v = preview()
	if v.Timezone != "UTC" || v.Original.Timezone != "UTC" || len(v.Commitments) != 1 {
		t.Fatal("existing plan zone reinterpreted", v)
	}
	get := readLowEnergy(t, f.privateRequest(t, a, "GET", "/recovery/low-energy/"+v.ID, "", nil))
	if get.SourceHash != v.SourceHash {
		t.Fatal("preview persisted incorrectly")
	}
	if _, e := f.pool.Exec(f.ctx, `CREATE FUNCTION reject_low_energy() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected final failure'; END $$; CREATE TRIGGER reject_low_energy BEFORE UPDATE ON low_energy_previews FOR EACH ROW EXECUTE FUNCTION reject_low_energy()`); e != nil {
		t.Fatal(e)
	}
	before := readPlan(t, f.privateRequest(t, a, "GET", "/plans/day/"+date, "", nil))
	if r := f.privateRequest(t, a, "POST", path(v.ID), "", apply); r.status != 500 {
		t.Fatal(r.status, string(r.body))
	}
	after := readPlan(t, f.privateRequest(t, a, "GET", "/plans/day/"+date, "", nil))
	if before.Version != after.Version || len(after.QuestIDs) != 3 {
		t.Fatal("partial plan apply escaped rollback")
	}
	if _, e := f.pool.Exec(f.ctx, "DROP TRIGGER reject_low_energy ON low_energy_previews; DROP FUNCTION reject_low_energy()"); e != nil {
		t.Fatal(e)
	}
	responses := make([]authResponse, 4)
	var wg sync.WaitGroup
	for i := range responses {
		wg.Add(1)
		go func(i int) { defer wg.Done(); responses[i] = f.privateRequest(t, a, "POST", path(v.ID), "", apply) }(i)
	}
	wg.Wait()
	var resultVersion int64
	for _, r := range responses {
		result := readLowEnergy(t, r)
		if resultVersion == 0 {
			resultVersion = result.Result.Version
		}
		if result.Result.Version != resultVersion || len(result.Result.QuestIDs) != 2 || len(result.Result.Snapshot.Items) != 3 {
			t.Fatal("concurrent apply duplicated or rewrote original", result)
		}
	}
}
