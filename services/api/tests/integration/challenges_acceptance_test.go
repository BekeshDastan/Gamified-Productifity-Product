//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestChallengePeriodicMissesAndCorrectionsPreserveFirstResult(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "challenge-periods@example.test")
	in := challengeInput(t, 10)
	in.TemplateID = nil
	in.Input.Name = "Два дня из трёх"
	in.Input.Kind = "periodic"
	in.Input.Period = "day"
	in.Input.PeriodTarget = 2
	in.Input.Target = 2
	in.Input.Days = 3
	in.Input.DailyCap = 2
	in.Input.AllowedMisses = 1
	in.Input.EndsOn = "2026-10-14"
	run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", in))
	path := "/challenges/runs/" + run.ID
	readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}))
	mark := readCheckin(t, f.privateRequest(t, a, "POST", path+"/check-ins", "", challenges.CheckinInput{OperationID: testkit.NewUser().ID, Unit: "count", Amount: 2}))
	f.clock.Advance(48 * time.Hour)
	f.loginAs(t, a, "challenge-periods@example.test")
	readCheckin(t, f.privateRequest(t, a, "POST", path+"/check-ins", "", challenges.CheckinInput{OperationID: testkit.NewUser().ID, Unit: "count", Amount: 2}))
	m := readChallengeMember(t, f.privateRequest(t, a, "GET", path+"/progress", "", nil))
	p := challengeProgress(t, m)
	if p.Current != 2 || !p.TargetMet || len(p.Periods) != 3 || p.Periods[1].State != "missed" {
		t.Fatal(p)
	}
	closed := readFinalization(t, f.privateRequest(t, a, "POST", path+"/close", `"1"`, challenges.CloseInput{Confirm: true}))
	if closed.Reward == nil || closed.Reward.AwardedXP != 25 {
		t.Fatal(closed)
	}
	readCheckin(t, f.privateRequest(t, a, "PATCH", path+"/check-ins/"+mark.ID, `"1"`, challenges.CheckinCorrection{Amount: 0, Note: "Уточнение"}))
	m = readChallengeMember(t, f.privateRequest(t, a, "GET", path+"/progress", "", nil))
	if challengeProgress(t, m).Current != 1 || *m.Result != "not_achieved" {
		t.Fatal(m)
	}
	readCheckin(t, f.privateRequest(t, a, "PATCH", path+"/check-ins/"+mark.ID, `"2"`, challenges.CheckinCorrection{Amount: 2, Note: "Восстановление"}))
	m = readChallengeMember(t, f.privateRequest(t, a, "GET", path+"/progress", "", nil))
	if challengeProgress(t, m).Current != 2 || m.Reward == nil || m.Reward.EntitlementID != closed.Reward.EntitlementID {
		t.Fatal(m)
	}
}
func TestChallengeSourceAndCheckinPaginationUseStableIDs(t *testing.T) {
	limits := auth.DefaultLimits()
	limits.UserRequests = 1000
	f := newAuthFixture(t, limits)
	a, _ := f.signedIn(t, "challenge-pages@example.test")
	in := challengeInput(t, 10)
	in.Input.Target = 101
	in.Input.DailyCap = 1000
	run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", in))
	path := "/challenges/runs/" + run.ID
	readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}))
	for i := 0; i < 101; i++ {
		readCheckin(t, f.privateRequest(t, a, "POST", path+"/check-ins", "", challenges.CheckinInput{OperationID: testkit.NewUser().ID, Unit: "count", Amount: 1, Note: fmt.Sprint(i)}))
	}
	var first, second struct{ Data challenges.SourcePage }
	r := f.privateRequest(t, a, "GET", path+"/sources", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &first) != nil || len(first.Data.Items) != 100 || first.Data.Next == "" || first.Data.Total != 101 {
		t.Fatal(r.status, string(r.body))
	}
	r = f.privateRequest(t, a, "GET", path+"/sources?before="+first.Data.Next, "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &second) != nil || len(second.Data.Items) != 1 || second.Data.Next != "" {
		t.Fatal(r.status, string(r.body))
	}
	for _, v := range first.Data.Items {
		if v.Key == second.Data.Items[0].Key {
			t.Fatal("overlapping pages")
		}
	}
	var marks struct{ Data challenges.CheckinPage }
	r = f.privateRequest(t, a, "GET", path+"/check-ins", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &marks) != nil || len(marks.Data.Items) != 50 || marks.Data.Next == "" {
		t.Fatal(r.status, string(r.body))
	}
	firstMark := marks.Data.Items[0].ID
	r = f.privateRequest(t, a, "GET", path+"/check-ins?before="+marks.Data.Next, "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &marks) != nil || len(marks.Data.Items) != 50 || marks.Data.Items[0].ID == firstMark {
		t.Fatal("check-in page overlap", r.status, string(r.body))
	}
	readFinalization(t, f.privateRequest(t, a, "POST", path+"/close", `"1"`, challenges.CloseInput{Confirm: true}))
	r = f.privateRequest(t, a, "GET", path+"/final-sources?before="+first.Data.Next, "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &second) != nil || len(second.Data.Items) != 1 {
		t.Fatal(r.status, string(r.body))
	}
	for _, suffix := range []string{"/sources?date=2026-10-11", "/sources?before=bad", "/check-ins?before=bad", "/final-sources?date=not-a-date"} {
		if r := f.privateRequest(t, a, "GET", path+suffix, "", nil); r.status != 422 && r.status != 404 {
			t.Fatal(suffix, r.status)
		}
	}
}
func TestChallengeManualMinutesValidationAndNoopCorrection(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "challenge-minutes@example.test")
	for _, change := range []func(*challenges.CreateInput){func(v *challenges.CreateInput) { v.Input.Name = "bad\x00name" }, func(v *challenges.CreateInput) { v.Input.Metric = "bad\x00metric" }, func(v *challenges.CreateInput) { id := "bad\x00template"; v.TemplateID = &id }} {
		bad := challengeInput(t, 9)
		change(&bad)
		if r := f.privateRequest(t, a, "POST", "/challenges/runs", "", bad); r.status != 422 {
			t.Fatal("unsafe JSON text", r.status, string(r.body))
		}
	}
	run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", challengeInput(t, 9)))
	path := "/challenges/runs/" + run.ID
	readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}))
	for _, bad := range []challenges.CheckinInput{{OperationID: "bad", Unit: "minutes", Amount: 2}, {OperationID: testkit.NewUser().ID, Unit: "pages", Amount: 2}, {OperationID: testkit.NewUser().ID, Unit: "minutes", Amount: 0}, {OperationID: testkit.NewUser().ID, Unit: "minutes", Amount: run.Rules.DailyCap + 1}, {OperationID: testkit.NewUser().ID, Unit: "minutes", Amount: 1, Note: "\x00"}} {
		if r := f.privateRequest(t, a, "POST", path+"/check-ins", "", bad); r.status != 422 {
			t.Fatal(r.status, string(r.body))
		}
	}
	v := readCheckin(t, f.privateRequest(t, a, "POST", path+"/check-ins", "", challenges.CheckinInput{OperationID: testkit.NewUser().ID, Unit: "minutes", Amount: 2}))
	same := readCheckin(t, f.privateRequest(t, a, "PATCH", path+"/check-ins/"+v.ID, `"1"`, challenges.CheckinCorrection{Amount: 2}))
	if same.Version != v.Version {
		t.Fatal("no-op correction changed version")
	}
	m := readChallengeMember(t, f.privateRequest(t, a, "GET", path+"/progress", "", nil))
	if challengeProgress(t, m).Current != 2 {
		t.Fatal("manual minutes scaled incorrectly", m)
	}
	if r := f.privateRequest(t, a, "GET", path+"/sources?before="+testkit.NewUser().ID, "", nil); r.status != 404 {
		t.Fatal("foreign cursor", r.status)
	}
}
func TestChallengeOwnerJobsContinueAndDuplicateSafely(t *testing.T) {
	limits := auth.DefaultLimits()
	limits.UserRequests = 1000
	f := newAuthFixture(t, limits)
	a, who := f.signedIn(t, "challenge-jobs@example.test")
	for i := 0; i < 12; i++ {
		run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", challengeInput(t, 2)))
		readChallengeMember(t, f.privateRequest(t, a, "POST", "/challenges/runs/"+run.ID+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}))
	}
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Общий источник")))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	svc := challenges.NewService(f.pool, f.clock).WithProgression(progression.NewService(f.pool, f.clock))
	job := jobs.Job{ID: testkit.NewUser().ID, OwnerID: &who.ID, Payload: []byte(`{}`)}
	handler := svc.Handlers()["challenges.changed"]
	process := func(j jobs.Job) {
		t.Helper()
		if e := storage.WithinTx(f.ctx, f.pool, func(tx pgx.Tx) error { return handler(f.ctx, tx, j) }); e != nil {
			t.Fatal(e)
		}
	}
	process(job)
	process(job)
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM challenge_participants WHERE owner_id=$1 AND computed_at IS NOT NULL", who.ID).Scan(&n); e != nil || n != 10 {
		t.Fatal(n, e)
	}
	var continuation jobs.Job
	continuation.OwnerID = &who.ID
	if e := f.pool.QueryRow(f.ctx, "SELECT id::text,payload FROM jobs WHERE owner_id=$1 AND kind='challenges.changed'", who.ID).Scan(&continuation.ID, &continuation.Payload); e != nil {
		t.Fatal(e)
	}
	process(continuation)
	process(continuation)
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM challenge_participants WHERE owner_id=$1 AND computed_at IS NOT NULL AND version=2 AND (progress->>'current')::int=1", who.ID).Scan(&n); e != nil || n != 12 {
		t.Fatal(n, e)
	}
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM reward_entitlements WHERE owner_id=$1 AND source_kind='challenge'", who.ID).Scan(&n); e != nil || n != 0 {
		t.Fatal("worker progress awarded XP", n, e)
	}
}
