//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"testing"
)

func TestEconomyVersionImmutable(t *testing.T) {
	pool, ctx := database(t)
	var data []byte
	if e := pool.QueryRow(ctx, `SELECT rules FROM economy_versions WHERE id=$1`, progression.Version).Scan(&data); e != nil {
		t.Fatal(e)
	}
	var rules struct {
		TaskXP       []int64 `json:"task_xp"`
		HabitXP      int64   `json:"habit_xp"`
		CoinsDivisor int64   `json:"coins_divisor"`
	}
	if e := json.Unmarshal(data, &rules); e != nil || len(rules.TaskXP) != 5 || rules.HabitXP != 10 || rules.CoinsDivisor != 5 {
		t.Fatal("catalog mismatch", string(data), e)
	}
	for i, xp := range rules.TaskXP {
		if xp != progression.TaskXP(i+1) {
			t.Fatal("Go and stored rules diverge")
		}
	}
	for _, query := range []string{`UPDATE economy_versions SET rules='{}' WHERE id='economy-v1'`, `DELETE FROM economy_versions WHERE id='economy-v1'`} {
		if _, e := pool.Exec(ctx, query); e == nil {
			t.Fatal("mutable economy version")
		}
	}
}
