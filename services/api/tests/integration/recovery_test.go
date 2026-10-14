//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/seasons"
	"sync"
	"testing"
	"time"
)

func readRecovery(t *testing.T, r authResponse) seasons.RecoveryEligibility {
	t.Helper()
	var out struct{ Data seasons.RecoveryEligibility }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func readRecoveryOffer(t *testing.T, r authResponse) seasons.RecoveryOffer {
	t.Helper()
	var out struct{ Data seasons.RecoveryOffer }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestRecoveryPreviewConsentConcurrentAcceptanceAndPermanentReplay(t *testing.T) {
	limits := auth.DefaultLimits()
	limits.UserRequests = 600
	f := newAuthFixture(t, limits)
	a, who := f.signedIn(t, "recovery-accept@example.test")
	initial := readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil))
	if initial.Eligible || initial.Offer != nil || initial.LastResultAt != nil {
		t.Fatal(initial)
	}
	f.clock.Advance(72*time.Hour - time.Second)
	f.loginAs(t, a, "recovery-accept@example.test")
	if readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil)).Eligible {
		t.Fatal("early recovery")
	}
	f.clock.Advance(time.Second)
	eligible := readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil))
	if !eligible.Eligible || eligible.Offer == nil || len(eligible.Offer.Steps) != 3 || eligible.Offer.CatalogVersion != seasons.RecoveryVersion || eligible.Offer.ChallengeRules.Target != 3 || eligible.Offer.ChallengeRules.Days != 7 || eligible.Offer.RewardXP != 25 || eligible.Offer.Version != 1 {
		t.Fatal(eligible)
	}
	again := readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil))
	if again.Offer.ID != eligible.Offer.ID {
		t.Fatal("duplicate offer")
	}
	var n int
	for _, table := range []string{"goals", "quests", "challenge_runs"} {
		if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table+" WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 0 {
			t.Fatal("unaccepted obligation", table, n, e)
		}
	}
	steps := append([]seasons.RecoveryStep(nil), eligible.Offer.Steps...)
	steps[0].Title = "Мой небольшой результат"
	steps[0].EstimatedMinutes = 1
	in := seasons.RecoveryDecision{Accept: true, Steps: steps, Timezone: eligible.Timezone}
	path := "/recovery/comeback/" + eligible.Offer.ID + "/decide"
	responses := make([]authResponse, 4)
	var wg sync.WaitGroup
	for i := range responses {
		wg.Add(1)
		go func(i int) { defer wg.Done(); responses[i] = f.privateRequest(t, a, "POST", path, `"1"`, in) }(i)
	}
	wg.Wait()
	accepted := readRecoveryOffer(t, responses[0])
	if accepted.State != "accepted" || accepted.Result == nil || accepted.Result.RunID == nil || accepted.Result.GoalID == nil || len(accepted.Result.QuestIDs) != 3 {
		t.Fatal(accepted)
	}
	for _, r := range responses {
		v := readRecoveryOffer(t, r)
		if *v.Result.RunID != *accepted.Result.RunID || *v.Result.GoalID != *accepted.Result.GoalID {
			t.Fatal("duplicate accepted bundle")
		}
	}
	for table, want := range map[string]int{"goals": 1, "quests": 3, "challenge_runs": 1, "challenge_participants": 1, "reward_ledger": 0} {
		if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table+" WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != want {
			t.Fatal(table, n, e)
		}
	}
	run := readChallenge(t, f.privateRequest(t, a, "GET", "/challenges/runs/"+*accepted.Result.RunID, "", nil))
	if run.Rules.Target != 3 || run.Rules.Days != 7 || run.Rules.GoalID == nil || *run.Rules.GoalID != *accepted.Result.GoalID {
		t.Fatal(run)
	}
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "recovery-accept@example.test")
	replay := readRecoveryOffer(t, f.privateRequest(t, a, "POST", path, `"1"`, in))
	if *replay.Result.RunID != *accepted.Result.RunID {
		t.Fatal("permanent decision replay")
	}
	if r := f.privateRequest(t, a, "POST", path, `"1"`, seasons.RecoveryDecision{Steps: []seasons.RecoveryStep{}}); r.status != 409 {
		t.Fatal("changed decision", r.status)
	}
	for _, id := range accepted.Result.QuestIDs {
		q := readQuest(t, f.privateRequest(t, a, "GET", "/quests/"+id, "", nil))
		if q.Status != "backlog" || q.PlannedDate != nil || q.Difficulty != 1 {
			t.Fatal("silent plan obligation", q)
		}
		readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+id+"/complete", version(q), quests.Transition{}))
	}
	m := readChallengeMember(t, f.privateRequest(t, a, "GET", "/challenges/runs/"+run.ID+"/progress", "", nil))
	if p := challengeProgress(t, m); p.Current != 3 || !p.TargetMet {
		t.Fatal(p)
	}
	result := readFinalization(t, f.privateRequest(t, a, "POST", "/challenges/runs/"+run.ID+"/close", `"1"`, challenges.CloseInput{Confirm: true}))
	if result.Reward == nil || result.Reward.AwardedXP != 25 {
		t.Fatal(result)
	}
	readRecoveryOffer(t, f.privateRequest(t, a, "POST", path, `"1"`, in))
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM quests WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 3 {
		t.Fatal("post-completion replay recreated tasks", n, e)
	}
}
func TestRecoveryDeclineHasNoPenaltyAndFourteenDayCooldown(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "recovery-decline@example.test")
	f.clock.Advance(72 * time.Hour)
	f.loginAs(t, a, "recovery-decline@example.test")
	eligible := readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil))
	path := "/recovery/comeback/" + eligible.Offer.ID + "/decide"
	in := seasons.RecoveryDecision{Steps: []seasons.RecoveryStep{}}
	declined := readRecoveryOffer(t, f.privateRequest(t, a, "POST", path, `"1"`, in))
	if declined.State != "declined" || declined.Result == nil || declined.Result.RunID != nil {
		t.Fatal(declined)
	}
	var n int
	for _, table := range []string{"quests", "goals", "challenge_runs", "reward_ledger"} {
		if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table+" WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 0 {
			t.Fatal("decline penalty or obligation", table, n, e)
		}
	}
	f.clock.Advance(13 * 24 * time.Hour)
	f.loginAs(t, a, "recovery-decline@example.test")
	if data := readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil)); data.Eligible || data.Offer.ID != declined.ID {
		t.Fatal(data)
	}
	f.clock.Advance(24 * time.Hour)
	f.loginAs(t, a, "recovery-decline@example.test")
	data := readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil))
	if !data.Eligible || data.Offer.ID == declined.ID {
		t.Fatal("cooldown never released", data)
	}
	readRecoveryOffer(t, f.privateRequest(t, a, "POST", path, `"1"`, in))
	var history struct{ Data seasons.RecoveryHistory }
	r := f.privateRequest(t, a, "GET", "/recovery/comeback/history", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &history) != nil || len(history.Data.Items) != 2 {
		t.Fatal(r.status, string(r.body))
	}
}
func TestRecoveryBundleRollbackAndChangedInactivity(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "recovery-atomic@example.test")
	b, _ := f.signedIn(t, "recovery-atomic-other@example.test")
	f.clock.Advance(72 * time.Hour)
	f.loginAs(t, a, "recovery-atomic@example.test")
	f.loginAs(t, b, "recovery-atomic-other@example.test")
	offer := readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil)).Offer
	path := "/recovery/comeback/" + offer.ID + "/decide"
	in := seasons.RecoveryDecision{Accept: true, Steps: offer.Steps, Timezone: "UTC"}
	if r := f.privateRequest(t, b, "POST", path, `"1"`, in); r.status != 404 {
		t.Fatal("foreign offer", r.status)
	}
	_, e := f.pool.Exec(f.ctx, `CREATE FUNCTION reject_recovery_accept() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='accepted' THEN RAISE EXCEPTION 'recovery fixture rollback'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_recovery_accept BEFORE UPDATE ON recovery_offers FOR EACH ROW EXECUTE FUNCTION reject_recovery_accept()`)
	if e != nil {
		t.Fatal(e)
	}
	key := "recovery-atomic-retry-0001"
	if r := f.privateRequest(t, a, "POST", path, `"1"`, in, key); r.status != 500 {
		t.Fatal("injected failure", r.status)
	}
	var n int
	for _, table := range []string{"goals", "quests", "challenge_runs", "challenge_participants"} {
		if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table+" WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 0 {
			t.Fatal("partial bundle survived", table, n, e)
		}
	}
	if _, e = f.pool.Exec(f.ctx, "DROP TRIGGER reject_recovery_accept ON recovery_offers; DROP FUNCTION reject_recovery_accept()"); e != nil {
		t.Fatal(e)
	}
	accepted := readRecoveryOffer(t, f.privateRequest(t, a, "POST", path, `"1"`, in, key))
	if accepted.Result == nil || len(accepted.Result.QuestIDs) != 3 {
		t.Fatal(accepted)
	}
	otherOffer := readRecovery(t, f.privateRequest(t, b, "GET", "/recovery/comeback", "", nil)).Offer
	f.clock.Advance(time.Minute)
	q := readQuest(t, f.privateRequest(t, b, "POST", "/quests", "", questInput("Уже вернулся")))
	readQuest(t, f.privateRequest(t, b, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	if r := f.privateRequest(t, b, "POST", "/recovery/comeback/"+otherOffer.ID+"/decide", fmt.Sprintf(`"%d"`, otherOffer.Version), seasons.RecoveryDecision{Accept: true, Steps: otherOffer.Steps, Timezone: "UTC"}); r.status != 409 {
		t.Fatal("stale inactivity accepted", r.status)
	}
	data := readRecovery(t, f.privateRequest(t, b, "GET", "/recovery/comeback", "", nil))
	if data.Eligible || data.Offer.State != "withdrawn" {
		t.Fatal("stale offer not withdrawn", data)
	}
}
func TestRecoveryTimezoneMustMatchReviewedContext(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "recovery-zone@example.test")
	f.clock.Advance(72 * time.Hour)
	f.loginAs(t, a, "recovery-zone@example.test")
	data := readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil))
	in := seasons.RecoveryDecision{Accept: true, Steps: data.Offer.Steps, Timezone: data.Timezone}
	path := "/recovery/comeback/" + data.Offer.ID + "/decide"
	if _, e := f.pool.Exec(f.ctx, "UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}',to_jsonb('Asia/Almaty'::text)),version=version+1 WHERE owner_id=$1", who.ID); e != nil {
		t.Fatal(e)
	}
	if r := f.privateRequest(t, a, "POST", path, `"1"`, in); r.status != 409 {
		t.Fatal("unreviewed timezone", r.status)
	}
	fresh := readRecovery(t, f.privateRequest(t, a, "GET", "/recovery/comeback", "", nil))
	if fresh.Offer.ID != data.Offer.ID || fresh.Timezone != "Asia/Almaty" {
		t.Fatal(fresh)
	}
	in.Timezone = fresh.Timezone
	accepted := readRecoveryOffer(t, f.privateRequest(t, a, "POST", path, `"1"`, in))
	run := readChallenge(t, f.privateRequest(t, a, "GET", "/challenges/runs/"+*accepted.Result.RunID, "", nil))
	if run.Rules.Timezone != in.Timezone {
		t.Fatal("accepted zone not frozen", run.Rules)
	}
}
