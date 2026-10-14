//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
	"time"
)

func readFinalization(t *testing.T, r authResponse) challenges.Finalization {
	t.Helper()
	var out struct{ Data challenges.Finalization }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestChallengeDeadlineEmptyLeftAndConsent(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "challenge-end-states@example.test")
	in := challengeInput(t, 2)
	in.Input.Target = 1
	run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", in))
	path := "/challenges/runs/" + run.ID
	if r := f.privateRequest(t, a, "POST", path+"/close", `"1"`, challenges.CloseInput{}); r.status != 422 {
		t.Fatal("close without consent", r.status)
	}
	m := readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}))
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Достигнуто, затем выход")))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	if r := f.privateRequest(t, a, "POST", path+"/close", `"2"`, challenges.CloseInput{Confirm: true}); r.status != 409 {
		t.Fatal("stale run close", r.status)
	}
	f.clock.Advance(time.Minute)
	readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/leave", fmt.Sprintf(`"%d"`, m.Version), challenges.LeaveInput{}))
	empty := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", challengeInput(t, 2)))
	f.clock.Advance(run.EndsAt.Sub(f.clock.Now()))
	f.loginAs(t, a, "challenge-end-states@example.test")
	svc := challenges.NewService(f.pool, f.clock).WithProgression(progression.NewService(f.pool, f.clock))
	for _, id := range []string{run.ID, empty.ID} {
		body, _ := json.Marshal(map[string]string{"run_id": id})
		e := storage.WithinTx(f.ctx, f.pool, func(tx pgx.Tx) error {
			return svc.Handlers()["challenges.close"](f.ctx, tx, jobs.Job{OwnerID: &who.ID, Payload: body})
		})
		if e != nil {
			t.Fatal(e)
		}
	}
	left := readFinalization(t, f.privateRequest(t, a, "POST", path+"/close", `"1"`, challenges.CloseInput{Confirm: true}))
	if left.Member == nil || left.Member.Result == nil || *left.Member.Result != "left" || left.Reward != nil {
		t.Fatal(left)
	}
	out := readFinalization(t, f.privateRequest(t, a, "POST", "/challenges/runs/"+empty.ID+"/close", `"1"`, challenges.CloseInput{Confirm: true}))
	if out.Run.State != "closed" || out.Member != nil || out.Reward != nil {
		t.Fatal(out)
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM reward_entitlements WHERE owner_id=$1 AND source_kind='challenge'", who.ID).Scan(&n); e != nil || n != 0 {
		t.Fatal("exit or timer awarded XP", n, e)
	}
}
func TestChallengeCloseWorkerAPIRaceCorrectionsAndFrozenSources(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "challenge-final@example.test")
	in := challengeInput(t, 2)
	in.Input.Target = 1
	run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", in))
	path := "/challenges/runs/" + run.ID
	readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}))
	if r := f.privateRequest(t, a, "POST", path+"/close", `"1"`, challenges.CloseInput{Confirm: true}); r.status != 409 {
		t.Fatal("early incomplete", r.status)
	}
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Исходный результат")))
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	f.clock.Advance(run.EndsAt.Sub(f.clock.Now()))
	f.loginAs(t, a, "challenge-final@example.test")
	svc := challenges.NewService(f.pool, f.clock).WithProgression(progression.NewService(f.pool, f.clock))
	store := jobs.Store{Pool: f.pool, Clock: f.clock}
	var e error
	var deadline *jobs.Job
	for i := 0; i < 100; i++ {
		job, err := store.Claim(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if job.Kind == "challenges.close" {
			deadline = &job
			break
		}
	}
	if deadline == nil {
		t.Fatal("deadline job missing")
	}
	var wg sync.WaitGroup
	var workerErr error
	results := make([]authResponse, 2)
	wg.Add(3)
	go func() { defer wg.Done(); workerErr = store.Process(f.ctx, *deadline, svc.Handlers()) }()
	for i := range results {
		go func(i int) {
			defer wg.Done()
			results[i] = f.privateRequest(t, a, "POST", path+"/close", `"1"`, challenges.CloseInput{Confirm: true})
		}(i)
	}
	wg.Wait()
	if workerErr != nil {
		t.Fatal(workerErr)
	}
	closed := readFinalization(t, results[0])
	readFinalization(t, results[1])
	if closed.Run.State != "closed" || closed.Member == nil || closed.Member.Result == nil || *closed.Member.Result != "achieved" || closed.Reward == nil || closed.Reward.AwardedXP != 25 {
		t.Fatal(closed)
	}
	memberSnapshot := append([]byte(nil), closed.Member.FinalSnapshot...)
	runSnapshot := append([]byte(nil), closed.Run.FinalSnapshot...)
	entitlement := closed.Reward.EntitlementID
	if e := store.Process(f.ctx, *deadline, svc.Handlers()); e != jobs.ErrLeaseLost {
		t.Fatal("stale worker lease", e)
	}
	var n int
	if e = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM reward_entitlements WHERE owner_id=$1 AND source_kind='challenge'", who.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	fresh := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("После закрытия")))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+fresh.ID+"/complete", version(fresh), quests.Transition{}))
	m := readChallengeMember(t, f.privateRequest(t, a, "GET", path+"/progress", "", nil))
	if p := challengeProgress(t, m); p.Current != 1 {
		t.Fatal("closed scope expanded", p)
	}
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/reopen", version(q), quests.Transition{Reason: "Исправление"}))
	m = readChallengeMember(t, f.privateRequest(t, a, "GET", path+"/progress", "", nil))
	if *m.Result != "not_achieved" || challengeProgress(t, m).Current != 0 || m.Reward == nil || m.Reward.EntitlementID != entitlement || m.Reward.AwardedXP != 25 {
		t.Fatal(m)
	}
	if !bytes.Equal(memberSnapshot, m.FinalSnapshot) {
		t.Fatal("original member snapshot overwritten")
	}
	currentRun := readChallenge(t, f.privateRequest(t, a, "GET", path, "", nil))
	if !bytes.Equal(runSnapshot, currentRun.FinalSnapshot) {
		t.Fatal("original run snapshot overwritten")
	}
	var finalSources struct{ Data challenges.SourcePage }
	r := f.privateRequest(t, a, "GET", path+"/final-sources", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &finalSources) != nil || finalSources.Data.Total != 1 || finalSources.Data.Items[0].Key != q.ID {
		t.Fatal(r.status, string(r.body))
	}
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	m = readChallengeMember(t, f.privateRequest(t, a, "GET", path+"/progress", "", nil))
	if *m.Result != "achieved" || m.Reward == nil || m.Reward.EntitlementID != entitlement || challengeProgress(t, m).Current != 1 {
		t.Fatal(m)
	}
	if _, e = f.pool.Exec(f.ctx, "UPDATE challenge_final_sources SET amount=amount+1 WHERE owner_id=$1", who.ID); e == nil {
		t.Fatal("mutable source snapshot")
	}
	if e = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM reward_ledger WHERE owner_id=$1 AND entitlement_id=$2", who.ID, entitlement).Scan(&n); e != nil || n != 3 {
		t.Fatal("duplicate reward or missing correction", n, e)
	}
}
func TestChallengeClosedManualQuotaAndNoResidualTomorrow(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "challenge-cap@example.test")
	var paths []string
	var first challenges.Checkin
	for i := 0; i < 3; i++ {
		in := challengeInput(t, 10)
		in.Input.Target = 1
		run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", in))
		path := "/challenges/runs/" + run.ID
		paths = append(paths, path)
		readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}))
		mark := readCheckin(t, f.privateRequest(t, a, "POST", path+"/check-ins", "", challenges.CheckinInput{OperationID: in.OperationID, Unit: "count", Amount: 1}))
		if i == 0 {
			first = mark
		}
		out := readFinalization(t, f.privateRequest(t, a, "POST", path+"/close", `"1"`, challenges.CloseInput{Confirm: true}))
		want := int64(25)
		if i == 2 {
			want = 0
		}
		if out.Reward == nil || out.Reward.AwardedXP != want {
			t.Fatal(out.Reward, want)
		}
	}
	readCheckin(t, f.privateRequest(t, a, "PATCH", paths[0]+"/check-ins/"+first.ID, `"1"`, challenges.CheckinCorrection{Amount: 0, Note: "Ошибка"}))
	m := readChallengeMember(t, f.privateRequest(t, a, "GET", paths[0]+"/progress", "", nil))
	if *m.Result != "not_achieved" || m.Reward == nil {
		t.Fatal(m)
	}
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "challenge-cap@example.test")
	out := readFinalization(t, f.privateRequest(t, a, "POST", paths[2]+"/close", `"1"`, challenges.CloseInput{Confirm: true}))
	if out.Reward == nil || out.Reward.AwardedXP != 0 {
		t.Fatal("quota residual leaked", out.Reward)
	}
	svc := progression.NewService(f.pool, f.clock)
	var balance progression.Balance
	if e := svc.WithinOwner(f.ctx, who.ID, func(tx pgx.Tx) error { var e error; balance, e = svc.Balance(f.ctx, tx, who.ID); return e }); e != nil || balance.TotalXP != 25 {
		t.Fatal(balance, e)
	}
}
