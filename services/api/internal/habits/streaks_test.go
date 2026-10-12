package habits

import (
	"testing"
	"time"
)

func TestScheduledStreaksAndCorrections(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	closed := func(n int, amount float64) Period {
		return Period{Target: 1, Amount: amount, End: now.Add(time.Duration(n) * 24 * time.Hour)}
	}
	p := []Period{closed(-5, 1), closed(-4, 1), {Excluded: true}, closed(-2, 1), closed(1, 0)}
	s := CalculateStats(p, now)
	if s.Current != 3 || s.Best != 3 || s.Missed != 0 {
		t.Fatal(s)
	}
	p[4].Amount = 1
	s = CalculateStats(p, now)
	if s.Current != 4 || s.Best != 4 {
		t.Fatal(s)
	}
	p[1].Amount = 0
	s = CalculateStats(p, now)
	if s.Current != 2 || s.Best != 2 || !s.Recovering || s.Missed != 1 {
		t.Fatal("correction failed", s)
	}
	p[4].Amount = 0
	s = CalculateStats(p, now.Add(48*time.Hour))
	if s.Current != 0 || s.Best != 1 || s.Recovering {
		t.Fatal("miss hidden", s)
	}
}
