package challenges

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"testing"
	"time"
)

func TestChallengeWorkersRejectMalformedPayloads(t *testing.T) {
	c := &platform.FixedClock{Time: time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)}
	s := NewService(nil, c).WithProgression(progression.NewService(nil, c))
	owner := "00000000-0000-4000-8000-000000000001"
	for kind, payloads := range map[string][]string{"challenges.changed": {"not-json", `{"after":"invalid"}`}, "challenges.close": {"not-json", `{}`, `{"run_id":"invalid"}`}} {
		handler := s.Handlers()[kind]
		if e := handler(context.Background(), nil, jobs.Job{}); !errors.Is(e, jobs.ErrPermanent) {
			t.Fatal(kind, e)
		}
		for _, body := range payloads {
			if e := handler(context.Background(), nil, jobs.Job{OwnerID: &owner, Payload: []byte(body)}); !errors.Is(e, jobs.ErrPermanent) {
				t.Fatal(kind, body, e)
			}
		}
	}
}
func TestFrozenChallengeRewardVersionCannotBeApproximated(t *testing.T) {
	s := NewService(nil, &platform.FixedClock{})
	body, _ := json.Marshal(FinalSnapshot{Kind: "result", Result: "achieved"})
	m := Member{State: "finished", FinalSnapshot: body}
	for _, r := range []Rules{{EconomyVersion: "future-version", RewardXP: 25}, {EconomyVersion: progression.Version, RewardXP: 100}} {
		if _, e := s.syncClosedReward(context.Background(), nil, "", Run{Rules: r}, m, Progress{TargetMet: true}); e == nil {
			t.Fatal("unknown economy accepted")
		}
	}
	m.FinalSnapshot = []byte(`{"kind":"cancelled"}`)
	if _, e := s.syncClosedReward(context.Background(), nil, "", Run{}, m, Progress{}); e != nil {
		t.Fatal(e)
	}
}
func TestScoringPartialWeekAndFrozenLocalDay(t *testing.T) {
	r := Rules{RuleInput: RuleInput{Config: Config{Metric: "manual", Unit: "count", Kind: "periodic", Period: "week", Target: 2, PeriodTarget: 1, Days: 8, DailyCap: 1}, StartsOn: "2026-10-12", EndsOn: "2026-10-19"}, Timezone: "Asia/Almaty"}
	p := calculate(r, []DayAmount{{"2026-10-12", 1}, {"2026-10-19", 1}}, 2, time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC))
	if p.Current != 2 || len(p.Periods) != 2 || p.Periods[1].Start != "2026-10-19" || p.Periods[1].End != "2026-10-19" {
		t.Fatal(p)
	}
	r.Period = "day"
	r.Target = 1
	p = calculate(r, nil, 0, time.Date(2026, 10, 12, 19, 0, 0, 0, time.UTC))
	if p.Periods[0].State != "missed" || p.Periods[1].State != "open" {
		t.Fatal(p)
	}
}
