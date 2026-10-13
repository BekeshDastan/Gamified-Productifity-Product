//go:build integration

package integration

import (
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
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
func TestProgressionTaskRewardsAreAtomicAndLeafOnly(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, account := f.signedIn(t, "task-reward@example.test")
	input := questInput("Листовая работа")
	input.Difficulty = 3
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	key := "task-reward-replay-00001"
	oldVersion := version(q)
	done := readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", oldVersion, map[string]string{"reason": "Готово"}, key))
	if done.Reward == nil || done.Reward.XPDelta != 25 || done.Reward.CoinsDelta != 5 || done.Reward.TotalXP != 25 {
		t.Fatal("completion reward", done.Reward)
	}
	repeat := readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", oldVersion, map[string]string{"reason": "Готово"}, key))
	if repeat.Reward == nil || repeat.Reward.EntitlementID != done.Reward.EntitlementID {
		t.Fatal("replay lost entitlement")
	}
	root := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Контейнер")))
	childInput := questInput("Обязательный лист")
	childInput.ParentID = &root.ID
	child := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", childInput))
	child = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+child.ID+"/complete", version(child), map[string]string{"reason": "Готово"}))
	root = readQuest(t, f.privateRequest(t, a, "GET", "/quests/"+root.ID, "", nil))
	if root.Status != "completed" || root.Reward != nil {
		t.Fatal("container result", root)
	}
	var count int
	var xp int64
	if e := f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM reward_entitlements WHERE owner_id=$1),(SELECT total_xp FROM user_progress WHERE owner_id=$1)`, account.ID).Scan(&count, &xp); e != nil || count != 2 || xp != 35 {
		t.Fatal("container or duplicate reward", count, xp, e)
	}
	var recorded int64
	if e := f.pool.QueryRow(t.Context(), `SELECT (after_state->'reward'->>'xp_delta')::bigint FROM task_events WHERE owner_id=$1 AND quest_id=$2 AND kind='complete'`, account.ID, q.ID).Scan(&recorded); e != nil || recorded != 25 {
		t.Fatal("history reward missing", recorded, e)
	}
}
func TestProgressionTaskAndLedgerRollbackTogether(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, account := f.signedIn(t, "task-ledger-rollback@example.test")
	input := questInput("Атомарная работа")
	input.Difficulty = 5
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	if _, e := f.pool.Exec(t.Context(), `CREATE FUNCTION reject_reward_ledger() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected economic failure'; END $$;CREATE TRIGGER reject_reward_ledger BEFORE INSERT ON reward_ledger FOR EACH ROW EXECUTE FUNCTION reject_reward_ledger()`); e != nil {
		t.Fatal(e)
	}
	key := "task-atomic-retry-00001"
	r := f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{"reason": "Готово"}, key)
	if r.status != 500 {
		t.Fatal(r.status, string(r.body))
	}
	fresh := readQuest(t, f.privateRequest(t, a, "GET", "/quests/"+q.ID, "", nil))
	var xp, count int64
	if e := f.pool.QueryRow(t.Context(), `SELECT total_xp,(SELECT count(*) FROM reward_entitlements WHERE owner_id=$1)+(SELECT count(*) FROM reward_quota_usage WHERE owner_id=$1)+(SELECT count(*) FROM reward_ledger WHERE owner_id=$1)+(SELECT count(*) FROM outbox_events WHERE owner_id=$1 AND kind='progression.reward_changed') FROM user_progress WHERE owner_id=$1`, account.ID).Scan(&xp, &count); e != nil || xp != 0 || count != 0 || fresh.Status != "todo" || fresh.Version != q.Version {
		t.Fatal("status and reward split", fresh, xp, count, e)
	}
	if _, e := f.pool.Exec(t.Context(), `DROP TRIGGER reject_reward_ledger ON reward_ledger;DROP FUNCTION reject_reward_ledger()`); e != nil {
		t.Fatal(e)
	}
	done := readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{"reason": "Готово"}, key))
	if done.Reward == nil || done.Reward.XPDelta != 100 {
		t.Fatal("retry after rollback", done.Reward)
	}
}

func TestProgressionHabitFullNormHasOneCanonicalReward(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, account := f.signedIn(t, "habit-reward@example.test")
	h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", habitInput()))
	path := "/habits/" + h.ID + "/logs"
	input := habits.LogInput{PeriodID: h.Periods[0].ID, Amount: 10, OccurredAt: f.clock.Now()}
	h = readHabit(t, f.privateRequest(t, a, "POST", path, habitVersion(h), input))
	if h.Reward != nil {
		t.Fatal("partial norm received XP", h.Reward)
	}
	key := "habit-complete-replay-00001"
	v := habitVersion(h)
	h = readHabit(t, f.privateRequest(t, a, "POST", path, v, input, key))
	if h.Reward == nil || h.Reward.XPDelta != 10 || h.Reward.CoinsDelta != 2 || h.Reward.SourceKey != input.PeriodID {
		t.Fatal("full period reward", h.Reward)
	}
	same := readHabit(t, f.privateRequest(t, a, "POST", path, v, input, key))
	if same.Reward == nil || same.Reward.EntitlementID != h.Reward.EntitlementID {
		t.Fatal("replay source changed")
	}
	h = readHabit(t, f.privateRequest(t, a, "POST", path, habitVersion(h), input))
	if h.Reward == nil || h.Reward.XPDelta != 0 || h.Periods[0].Amount != 30 {
		t.Fatal("extra norm reward", h.Reward)
	}
	var count, xp int64
	if e := f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM reward_ledger WHERE owner_id=$1),total_xp FROM user_progress WHERE owner_id=$1`, account.ID).Scan(&count, &xp); e != nil || count != 1 || xp != 10 {
		t.Fatal(count, xp, e)
	}
}
func TestProgressionHabitLateWindowBoundary(t *testing.T) {
	for _, offset := range []time.Duration{0, time.Second} {
		t.Run(offset.String(), func(t *testing.T) {
			f := newAuthFixture(t, auth.DefaultLimits())
			a, account := f.signedIn(t, "habit-late@example.test")
			in := habitInput()
			in.StartsOn = "2026-10-11"
			in.Schedule.Weekdays = []int{1, 2, 3, 4, 5, 6, 7}
			h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", in))
			p := h.Periods[0]
			deadline := p.End.Add(24 * time.Hour)
			f.clock.Advance(deadline.Add(offset).Sub(f.clock.Now()))
			if r := f.request(t, a, "POST", "/refresh", f.csrf(t, a), struct{}{}); r.status != 200 {
				t.Fatal(r.status)
			}
			h = readHabit(t, f.privateRequest(t, a, "POST", "/habits/"+h.ID+"/logs", habitVersion(h), habits.LogInput{PeriodID: p.ID, Amount: 20, OccurredAt: p.Start.Add(time.Hour)}))
			if h.Reward == nil {
				t.Fatal("missing reward explanation")
			}
			var xp int64
			if e := f.pool.QueryRow(t.Context(), `SELECT total_xp FROM user_progress WHERE owner_id=$1`, account.ID).Scan(&xp); e != nil {
				t.Fatal(e)
			}
			want := int64(10)
			if offset > 0 {
				want = 0
				if h.Reward.Kind != "not_awarded" {
					t.Fatal(h.Reward)
				}
			}
			if xp != want || h.Reward.XPDelta != want || h.Periods[0].Amount != 20 {
				t.Fatal("late history or reward", h.Reward, h.Periods[0])
			}
		})
	}
}
func TestProgressionHabitLogAndRewardRollbackTogether(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, account := f.signedIn(t, "habit-reward-rollback@example.test")
	h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", habitInput()))
	if _, e := f.pool.Exec(t.Context(), `CREATE FUNCTION reject_habit_reward() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected habit reward failure'; END $$;CREATE TRIGGER reject_habit_reward BEFORE INSERT ON reward_ledger FOR EACH ROW EXECUTE FUNCTION reject_habit_reward()`); e != nil {
		t.Fatal(e)
	}
	input := habits.LogInput{PeriodID: h.Periods[0].ID, Amount: 20, OccurredAt: f.clock.Now()}
	r := f.privateRequest(t, a, "POST", "/habits/"+h.ID+"/logs", habitVersion(h), input)
	if r.status != 500 {
		t.Fatal(r.status)
	}
	fresh := readHabit(t, f.privateRequest(t, a, "GET", "/habits/"+h.ID, "", nil))
	var logs, xp int64
	if e := f.pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM habit_logs WHERE owner_id=$1),total_xp FROM user_progress WHERE owner_id=$1`, account.ID).Scan(&logs, &xp); e != nil || logs != 0 || xp != 0 || fresh.Version != h.Version || fresh.Periods[0].Amount != 0 {
		t.Fatal("habit and reward split", logs, xp, fresh, e)
	}
}
