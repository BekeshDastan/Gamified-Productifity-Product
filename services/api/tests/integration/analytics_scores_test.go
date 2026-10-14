//go:build integration

package integration

import (
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/habits"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"testing"
	"time"
)

func TestAnalyticsDaily65UsesOriginalConfirmationAndHabitNorm(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "analytics-65@example.test")
	date := "2026-10-12"
	ids := []string{}
	var first quests.Quest
	for _, priority := range []string{"high", "low"} {
		in := questInput(priority)
		in.Priority = priority
		in.EstimatedMinutes = 10
		q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", in))
		ids = append(ids, q.ID)
		if first.ID == "" {
			first = q
		}
	}
	p := readPlan(t, f.privateRequest(t, a, "PUT", "/plans/day/"+date, `"0"`, planning.Input{AvailableMinutes: 20, QuestIDs: ids}))
	p = readPlan(t, f.privateRequest(t, a, "POST", "/plans/day/"+date+"/confirm", planVersion(p), struct{}{}))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+first.ID+"/complete", version(first), quests.Transition{}))
	h := readHabit(t, f.privateRequest(t, a, "POST", "/habits", "", habitInput()))
	readHabit(t, f.privateRequest(t, a, "POST", "/habits/"+h.ID+"/logs", habitVersion(h), habits.LogInput{PeriodID: h.Periods[0].ID, Amount: 10, OccurredAt: f.clock.Now()}))
	// A lighter current plan cannot repair the original denominator or R snapshot.
	readPlan(t, f.privateRequest(t, a, "POST", "/plans/day/"+date+"/addendum", planVersion(p), planning.Amendment{Input: planning.Input{AvailableMinutes: 100, QuestIDs: []string{first.ID}}, Reason: "Облегчаю текущий список"}))
	f.clock.Advance(24 * time.Hour)
	f.loginAs(t, a, "analytics-65@example.test")
	v := readAnalytics(t, f.privateRequest(t, a, "POST", "/analytics/rebuild", "", analytics.Range{From: date, To: "2026-10-13"}))
	d := v.Days[0]
	if d.Scores.Daily == nil || *d.Scores.Daily != 65 || *d.Scores.T != .75 || *d.Scores.H != .5 || d.Planned != 2 || d.Plan.Input.AvailableMinutes != 20 || d.Plan.Snapshot == nil {
		t.Fatal(d)
	}
	if _, e := f.pool.Exec(f.ctx, "UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}',to_jsonb('Asia/Almaty'::text)),version=version+1 WHERE owner_id=$1", who.ID); e != nil {
		t.Fatal(e)
	}
	v = readAnalytics(t, f.privateRequest(t, a, "POST", "/analytics/rebuild", "", analytics.Range{From: date, To: "2026-10-13"}))
	if v.Days[0].Scores.Daily == nil || *v.Days[0].Scores.Daily != 65 || v.Days[0].Plan.Timezone != "UTC" || v.Timezone != "Asia/Almaty" {
		t.Fatal("frozen plan reinterpreted", v)
	}
	var definition string
	if e := f.pool.QueryRow(f.ctx, "SELECT version FROM score_versions WHERE version=$1", analytics.ScoreVersion).Scan(&definition); e != nil || definition != analytics.ScoreVersion {
		t.Fatal("missing formula version", e)
	}
}
