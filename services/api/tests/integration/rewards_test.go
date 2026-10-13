//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
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
