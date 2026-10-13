//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/rewards"
	"testing"
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
