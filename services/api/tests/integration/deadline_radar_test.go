//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/calendar"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
	"testing"
	"time"
)

func TestDeadlineRadarBucketsKnownCapacityAndOwnerScope(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "radar@example.test")
	b, _ := f.signedIn(t, "radar-other@example.test")
	now := f.clock.Now()
	for _, v := range []struct {
		name     string
		duration time.Duration
		minutes  int
	}{{"Просрочена", -time.Hour, 10}, {"Скоро", 2 * time.Hour, 20}, {"Без оценки", 48 * time.Hour, 0}, {"На неделе", 6 * 24 * time.Hour, 30}, {"Далеко", 8 * 24 * time.Hour, 100}} {
		in := questInput(v.name)
		in.EstimatedMinutes = v.minutes
		deadline := now.Add(v.duration)
		in.Deadline = &deadline
		readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", in))
	}
	readPlan(t, f.privateRequest(t, a, "PUT", "/plans/day/2026-10-12", `"0"`, planning.Input{AvailableMinutes: 60, Windows: []planning.Window{{Start: "13:00", End: "14:00"}}}))
	readEvent(t, f.privateRequest(t, a, "POST", "/calendar/events", "", calendar.Input{Title: "Занято", Start: "2026-10-12T13:30:00Z", End: "2026-10-12T14:00:00Z", Timezone: "UTC"}))
	read := func(r authResponse) planning.Radar {
		var out struct{ Data planning.Radar }
		if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
			t.Fatal(r.status, string(r.body))
		}
		return out.Data
	}
	radar := read(f.privateRequest(t, a, "GET", "/plans/deadline-radar", "", nil))
	if len(radar.Buckets) != 4 || len(radar.Buckets[0].Items) != 1 || len(radar.Buckets[1].Items) != 1 || radar.Buckets[1].FreeMinutes != 30 || radar.Buckets[1].CumulativeMinutes != 30 || radar.Buckets[2].UnknownEstimates != 1 || radar.Buckets[3].DeficitMinutes != 30 {
		t.Fatal(radar)
	}
	other := read(f.privateRequest(t, b, "GET", "/plans/deadline-radar", "", nil))
	for _, v := range other.Buckets {
		if len(v.Items) > 0 || v.RequiredMinutes != 0 || v.FreeMinutes != 0 {
			t.Fatal("foreign or invented capacity", v)
		}
	}
}
