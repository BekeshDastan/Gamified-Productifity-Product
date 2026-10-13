//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/rewards"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"sync"
	"testing"
	"time"
)

func readRewardDefinition(t *testing.T, r authResponse) rewards.Definition {
	t.Helper()
	var out struct{ Data rewards.Definition }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatalf("reward %d %s", r.status, r.body)
	}
	return out.Data
}
func rewardVersion(d rewards.Definition) string { return fmt.Sprintf(`"%d"`, d.Version) }
func TestRewardDefinitionPriceConfirmationArchiveAndPrivacy(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "reward-def-a@example.test")
	b, _ := f.signedIn(t, "reward-def-b@example.test")
	in := rewards.DefinitionInput{Title: "Вечер игры", Cost: 5, Available: true}
	d := readRewardDefinition(t, f.privateRequest(t, a, "POST", "/rewards/custom", "", in))
	in.Cost = -1
	if r := f.privateRequest(t, a, "PATCH", "/rewards/custom/"+d.ID, rewardVersion(d), in); r.status != 422 {
		t.Fatal("negative", r.status)
	}
	in.Cost = 10
	if r := f.privateRequest(t, a, "PATCH", "/rewards/custom/"+d.ID, rewardVersion(d), in); r.status != 422 {
		t.Fatal("missing confirmation", r.status)
	}
	in.PriceConfirmed = true
	if r := f.privateRequest(t, b, "PATCH", "/rewards/custom/"+d.ID, rewardVersion(d), in); r.status != 404 {
		t.Fatal("foreign", r.status)
	}
	in.Archived = true
	updated := readRewardDefinition(t, f.privateRequest(t, a, "PATCH", "/rewards/custom/"+d.ID, rewardVersion(d), in))
	if updated.Cost != 10 || !updated.Archived || updated.Version != 2 {
		t.Fatal(updated)
	}
	var n int
	if e := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM custom_reward_versions WHERE reward_id=$1", d.ID).Scan(&n); e != nil || n != 2 {
		t.Fatal("snapshot history", n, e)
	}
}

func readRedemption(t *testing.T, r authResponse) rewards.Redemption {
	t.Helper()
	var out struct{ Data rewards.Redemption }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatalf("redemption %d %s", r.status, r.body)
	}
	return out.Data
}
func TestRewardPurchaseRacePermanentReplayAndPriceSnapshot(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, owner := f.signedIn(t, "purchase-race@example.test")
	in := rewards.DefinitionInput{Title: "Вечер игры", Description: "Одна покупка", Cost: 5, Available: true}
	d := readRewardDefinition(t, f.privateRequest(t, a, "POST", "/rewards/custom", "", in))
	qIn := questInput("Заработать пять монет")
	qIn.Difficulty = 3
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", qIn))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{}))
	inputs := []rewards.PurchaseInput{{OperationID: testkit.NewUser().ID, Kind: "custom", TargetID: d.ID, ExpectedCost: 5, ExpectedVersion: 1}, {OperationID: testkit.NewUser().ID, Kind: "custom", TargetID: d.ID, ExpectedCost: 5, ExpectedVersion: 1}}
	var wg sync.WaitGroup
	responses := make([]authResponse, 2)
	for i := range inputs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			responses[i] = f.privateRequest(t, a, "POST", "/rewards/purchase", "", inputs[i])
		}(i)
	}
	wg.Wait()
	winner := -1
	for i, r := range responses {
		if r.status == 200 {
			if winner >= 0 {
				t.Fatal("double spend")
			}
			winner = i
		} else if r.status != 409 {
			t.Fatal(r.status, string(r.body))
		}
	}
	if winner < 0 {
		t.Fatal("no purchase")
	}
	purchase := readRedemption(t, responses[winner])
	var coins, n int64
	if e := f.pool.QueryRow(t.Context(), "SELECT coins,(SELECT count(*) FROM reward_redemptions WHERE owner_id=$1) FROM user_progress WHERE owner_id=$1", owner.ID).Scan(&coins, &n); e != nil || coins != 0 || n != 1 {
		t.Fatal(coins, n, e)
	}
	in.Cost = 10
	in.PriceConfirmed = true
	readRewardDefinition(t, f.privateRequest(t, a, "PATCH", "/rewards/custom/"+d.ID, rewardVersion(d), in))
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "purchase-race@example.test")
	replayed := readRedemption(t, f.privateRequest(t, a, "POST", "/rewards/purchase", "", inputs[winner]))
	if replayed.ID != purchase.ID || replayed.Cost != 5 || replayed.Title != "Вечер игры" {
		t.Fatal("lost purchase snapshot", replayed)
	}
	inputs[winner].ExpectedCost = 10
	if r := f.privateRequest(t, a, "POST", "/rewards/purchase", "", inputs[winner]); r.status != 409 {
		t.Fatal("permanent operation mismatch", r.status)
	}
}
func TestRewardPurchasedCosmeticSurvivesSpentXPCorrection(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "cosmetic-purchase@example.test")
	var ids []string
	var versions []string
	for range 2 {
		in := questInput("Эпическая задача")
		in.Difficulty = 5
		q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", in))
		q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{}))
		ids = append(ids, q.ID)
		versions = append(versions, version(q))
	}
	in := rewards.PurchaseInput{OperationID: testkit.NewUser().ID, Kind: "cosmetic", TargetID: "base-3", ExpectedCost: 30, ExpectedVersion: 1}
	readRedemption(t, f.privateRequest(t, a, "POST", "/rewards/purchase", "", in))
	in.OperationID = testkit.NewUser().ID
	if r := f.privateRequest(t, a, "POST", "/rewards/purchase", "", in); r.status != 409 {
		t.Fatal("duplicate cosmetic purchase", r.status)
	}
	for i, id := range ids {
		readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+id+"/reopen", versions[i], map[string]string{}))
	}
	p := readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
	owned := false
	for _, item := range p.Inventory {
		if item.ID == "base-3" {
			owned = item.Owned
		}
	}
	if !owned {
		t.Fatal("paid ownership revoked by level correction")
	}
}

func TestRewardCancelUsesOriginalCostOnceAndClaimCannotRefund(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, owner := f.signedIn(t, "reward-return@example.test")
	in := rewards.DefinitionInput{Title: "Отдых", Cost: 5, Available: true}
	d := readRewardDefinition(t, f.privateRequest(t, a, "POST", "/rewards/custom", "", in))
	qi := questInput("Работа")
	qi.Difficulty = 3
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", qi))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{}))
	buy := func() rewards.Redemption {
		return readRedemption(t, f.privateRequest(t, a, "POST", "/rewards/purchase", "", rewards.PurchaseInput{OperationID: testkit.NewUser().ID, Kind: "custom", TargetID: d.ID, ExpectedCost: d.Cost, ExpectedVersion: d.Version}))
	}
	purchase := buy()
	in.Cost = 10
	in.PriceConfirmed = true
	d = readRewardDefinition(t, f.privateRequest(t, a, "PATCH", "/rewards/custom/"+d.ID, rewardVersion(d), in))
	path := "/rewards/redemptions/" + purchase.ID
	originalVersion := fmt.Sprintf(`"%d"`, purchase.Version)
	if _, e := f.pool.Exec(t.Context(), "CREATE FUNCTION reject_refund() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='refund' THEN RAISE EXCEPTION 'injected refund failure';END IF;RETURN NEW;END $$;CREATE TRIGGER reject_refund BEFORE INSERT ON reward_ledger FOR EACH ROW EXECUTE FUNCTION reject_refund()"); e != nil {
		t.Fatal(e)
	}
	key := "cancel-after-rollback-001"
	if r := f.privateRequest(t, a, "POST", path+"/cancel", originalVersion, map[string]string{}, key); r.status != 500 {
		t.Fatal("injection", r.status)
	}
	var coins int64
	var status string
	if e := f.pool.QueryRow(t.Context(), "SELECT coins,(SELECT status FROM reward_redemptions WHERE owner_id=$1 AND id=$2) FROM user_progress WHERE owner_id=$1", owner.ID, purchase.ID).Scan(&coins, &status); e != nil || coins != 0 || status != "pending" {
		t.Fatal("partial cancellation", coins, status, e)
	}
	if _, e := f.pool.Exec(t.Context(), "DROP TRIGGER reject_refund ON reward_ledger;DROP FUNCTION reject_refund()"); e != nil {
		t.Fatal(e)
	}
	cancelled := readRedemption(t, f.privateRequest(t, a, "POST", path+"/cancel", originalVersion, map[string]string{}, key))
	readRedemption(t, f.privateRequest(t, a, "POST", path+"/cancel", originalVersion, map[string]string{}))
	var refunds int64
	if e := f.pool.QueryRow(t.Context(), "SELECT coins,(SELECT count(*) FROM reward_ledger WHERE owner_id=$1 AND kind='refund') FROM user_progress WHERE owner_id=$1", owner.ID).Scan(&coins, &refunds); e != nil || coins != 5 || refunds != 1 || cancelled.Cost != 5 {
		t.Fatal("duplicate or repriced refund", coins, refunds, cancelled, e)
	}
	if r := f.privateRequest(t, a, "POST", path+"/claim", fmt.Sprintf(`"%d"`, cancelled.Version), map[string]string{}); r.status != 409 {
		t.Fatal("cancelled claim", r.status)
	}
	in.Cost = 5
	d = readRewardDefinition(t, f.privateRequest(t, a, "PATCH", "/rewards/custom/"+d.ID, rewardVersion(d), in))
	purchase = buy()
	claimed := readRedemption(t, f.privateRequest(t, a, "POST", "/rewards/redemptions/"+purchase.ID+"/claim", fmt.Sprintf(`"%d"`, purchase.Version), map[string]string{}))
	if r := f.privateRequest(t, a, "POST", "/rewards/redemptions/"+purchase.ID+"/cancel", fmt.Sprintf(`"%d"`, claimed.Version), map[string]string{}); r.status != 409 {
		t.Fatal("claimed refund", r.status)
	}
}

func readBoss(t *testing.T, r authResponse) rewards.Boss {
	t.Helper()
	var result struct{ Data rewards.Boss }
	if r.status != 200 || json.Unmarshal(r.body, &result) != nil {
		t.Fatalf("boss %d %s", r.status, r.body)
	}
	return result.Data
}
func TestBossHPVictoryCorrectionAndNoDuplicateXP(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, owner := f.signedIn(t, "boss-source@example.test")
	g := readGoal(t, f.privateRequest(t, a, "POST", "/goals", "", goals.Create{Input: goalInput("Большой результат"), State: "active"}))
	if r := f.privateRequest(t, a, "POST", "/bosses", "", map[string]string{"goal_id": g.ID}); r.status != 422 {
		t.Fatal("missing required stages", r.status)
	}
	qi := questInput("Обязательный шаг")
	qi.Difficulty = 3
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", qi))
	g = readGoal(t, f.privateRequest(t, a, "PUT", "/goals/"+g.ID+"/links/"+q.ID, goalVersion(g), goals.LinkInput{Required: true, Weight: 1}))
	b := readBoss(t, f.privateRequest(t, a, "POST", "/bosses", "", map[string]string{"goal_id": g.ID}))
	if b.HP != 100 || len(b.Stages) != 1 {
		t.Fatal(b)
	}
	get := func() rewards.Boss {
		r := f.privateRequest(t, a, "GET", "/bosses", "", nil)
		var out struct{ Data []rewards.Boss }
		if r.status != 200 || json.Unmarshal(r.body, &out) != nil || len(out.Data) != 1 {
			t.Fatal(string(r.body))
		}
		return out.Data[0]
	}
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{}))
	if b = get(); b.HP != 0 || b.Status != "awaiting_confirmation" {
		t.Fatal("premature victory", b)
	}
	g = readGoal(t, f.privateRequest(t, a, "GET", "/goals/"+g.ID, "", nil))
	readGoal(t, f.privateRequest(t, a, "POST", "/goals/"+g.ID+"/achieve", goalVersion(g), goals.Transition{Reason: "Проверил критерий", CriteriaConfirmed: true}))
	if b = get(); b.Status != "victory" || b.WonAt == nil {
		t.Fatal(b)
	}
	var xp int64
	if e := f.pool.QueryRow(t.Context(), "SELECT total_xp FROM user_progress WHERE owner_id=$1", owner.ID).Scan(&xp); e != nil || xp != 25 {
		t.Fatal("boss doubled XP", xp, e)
	}
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/reopen", version(q), map[string]string{}))
	if b = get(); b.HP != 100 || b.Status == "victory" {
		t.Fatal("asymmetric HP", b)
	}
	p := readCharacter(t, f.privateRequest(t, a, "GET", "/character", "", nil))
	for _, title := range p.Titles {
		if title.ID == "boss:victory" && title.Earned {
			t.Fatal("corrected victory title remains")
		}
	}
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{}))
	g = readGoal(t, f.privateRequest(t, a, "GET", "/goals/"+g.ID, "", nil))
	readGoal(t, f.privateRequest(t, a, "POST", "/goals/"+g.ID+"/achieve", goalVersion(g), goals.Transition{Reason: "Восстановил подтверждение", CriteriaConfirmed: true}))
	get()
	var grants int
	if e := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM cosmetic_permanent_grants WHERE owner_id=$1 AND source_kind='boss' AND active", owner.ID).Scan(&grants); e != nil || grants != 1 {
		t.Fatal("duplicate victory grant", grants, e)
	}
}
