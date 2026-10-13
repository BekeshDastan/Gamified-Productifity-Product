//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"testing"
)

func TestChallengeCatalogIsValidatedAndCreatesNoObligations(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "challenge-catalog@example.test")
	r := f.privateRequest(t, a, "GET", "/challenges/catalog", "", nil)
	var out struct{ Data challenges.Catalog }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil || len(out.Data.Challenges) != 12 || len(out.Data.Quests) != 20 {
		t.Fatal(r.status, string(r.body))
	}
	for _, template := range out.Data.Challenges {
		if e := challenges.ValidateConfig(template.Config); e != nil {
			t.Fatal(e)
		}
	}
	var n int
	for _, table := range []string{"quests", "day_plans"} {
		if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM "+table+" WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 0 {
			t.Fatal("catalog invented obligation", table, n, e)
		}
	}
}
