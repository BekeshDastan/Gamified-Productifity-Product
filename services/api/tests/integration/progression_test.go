//go:build integration

package integration

import (
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"github.com/jackc/pgx/v5"
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

func TestProgressionLedgerQuotaSnapshotAndWindow(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	_, account := f.signedIn(t, "ledger@example.test")
	svc := progression.NewService(f.pool, f.clock)
	award := func(source progression.Source) *progression.Reward {
		var out *progression.Reward
		e := svc.WithinOwner(t.Context(), account.ID, func(tx pgx.Tx) error {
			var err error
			out, err = svc.Award(t.Context(), tx, account.ID, source)
			return err
		})
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	var last progression.Source
	for _, difficulty := range []int{5, 4, 3, 2, 2, 5} {
		last = progression.Source{Kind: "task", Key: testkit.NewUser().ID, Difficulty: difficulty, Complete: true, Eligible: true, AllowFirst: true}
		r := award(last)
		if r == nil {
			t.Fatal("missing award")
		}
	}
	r := award(last)
	if r.Kind != "unchanged" || r.XPDelta != 0 || r.TotalXP != 200 || r.AwardedXP != 5 || r.AwardedCoins != 1 {
		t.Fatal("cap or duplicate", r)
	}
	last.Difficulty = 1
	r = award(last)
	if r.RequestedXP != 100 || r.AwardedXP != 5 {
		t.Fatal("mutable source snapshot", r)
	}
	if _, e := f.pool.Exec(t.Context(), `UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}',to_jsonb('Pacific/Kiritimati'::text)) WHERE owner_id=$1`, account.ID); e != nil {
		t.Fatal(e)
	}
	zero := award(progression.Source{Kind: "task", Key: testkit.NewUser().ID, Difficulty: 5, Complete: true, Eligible: true, AllowFirst: true})
	if zero.AwardedXP != 0 || zero.TotalXP != 200 {
		t.Fatal("timezone reset quota", zero)
	}
	for i := 0; i < 5; i++ {
		award(progression.Source{Kind: "habit", Key: testkit.NewUser().ID, Complete: true, Eligible: true, AllowFirst: true})
	}
	for i := 0; i < 2; i++ {
		award(progression.Source{Kind: "challenge", Key: testkit.NewUser().ID, Complete: true, Eligible: true, AllowFirst: true})
	}
	var before progression.Balance
	if e := svc.WithinOwner(t.Context(), account.ID, func(tx pgx.Tx) error {
		var err error
		before, err = svc.Balance(t.Context(), tx, account.ID)
		return err
	}); e != nil {
		t.Fatal(e)
	}
	if before.TotalXP != 300 || before.Coins != 60 || before.Quota.TotalUsed != 300 {
		t.Fatal(before)
	}
	f.clock.Advance(24 * 60 * 60 * 1e9)
	r = award(last)
	if r.AwardedXP != 5 || r.XPDelta != 0 {
		t.Fatal("residual carried tomorrow", r)
	}
	r = award(progression.Source{Kind: "task", Key: testkit.NewUser().ID, Difficulty: 5, Complete: true, Eligible: true, AllowFirst: true})
	if r.XPDelta != 100 || r.TotalXP != 400 {
		t.Fatal("window not released", r)
	}
	for _, query := range []string{`UPDATE reward_ledger SET xp_delta=1 WHERE owner_id=$1`, `DELETE FROM reward_ledger WHERE owner_id=$1`, `UPDATE reward_entitlements SET awarded_xp=0 WHERE owner_id=$1`, `UPDATE reward_quota_usage SET xp=0 WHERE owner_id=$1`} {
		if _, e := f.pool.Exec(t.Context(), query, account.ID); e == nil {
			t.Fatal("mutable economic history", query)
		}
	}
	var xp, coins int64
	if e := f.pool.QueryRow(t.Context(), `SELECT sum(xp_delta),sum(coins_delta) FROM reward_ledger WHERE owner_id=$1`, account.ID).Scan(&xp, &coins); e != nil || xp != 400 || coins != 80 {
		t.Fatal("ledger versus projection", xp, coins, e)
	}
}
func TestProgressionRollbackAndNonRewardSources(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	_, account := f.signedIn(t, "ledger-rollback@example.test")
	svc := progression.NewService(f.pool, f.clock)
	source := progression.Source{Kind: "task", Key: testkit.NewUser().ID, Difficulty: 3, Complete: true, Eligible: true, AllowFirst: true}
	sentinel := errors.New("rollback after ledger")
	e := svc.WithinOwner(t.Context(), account.ID, func(tx pgx.Tx) error {
		if _, err := svc.Award(t.Context(), tx, account.ID, source); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	var n int
	if e = f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM reward_entitlements)+(SELECT count(*) FROM reward_ledger)+(SELECT count(*) FROM reward_quota_usage)+(SELECT count(*) FROM outbox_events WHERE kind='progression.reward_changed')`).Scan(&n); e != nil || n != 0 {
		t.Fatal("partial economic commit", n, e)
	}
	for _, flags := range [][3]bool{{false, true, true}, {true, false, true}, {true, true, false}} {
		in := source
		in.Complete = flags[0]
		in.Eligible = flags[1]
		in.AllowFirst = flags[2]
		e = svc.WithinOwner(t.Context(), account.ID, func(tx pgx.Tx) error {
			r, err := svc.Award(t.Context(), tx, account.ID, in)
			if r != nil {
				t.Fatal("container/partial/late got reward")
			}
			return err
		})
		if e != nil {
			t.Fatal(e)
		}
	}
}
