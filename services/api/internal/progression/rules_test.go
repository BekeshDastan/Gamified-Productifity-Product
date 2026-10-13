package progression

import "testing"

func TestLevelThresholdsAndLargeIntegers(t *testing.T) {
	for _, c := range []struct{ xp, want int64 }{{-1, 1}, {0, 1}, {99, 1}, {100, 2}, {399, 2}, {400, 3}, {899, 3}, {900, 4}, {1000000000000, 100001}} {
		v := LevelFor(c.xp)
		if v.Number != c.want || v.Threshold > max(0, c.xp) || v.NextThreshold <= max(0, c.xp) || v.ToNext <= 0 {
			t.Fatal(c, v)
		}
	}
	for _, xp := range []int64{MaxSafeXP - 1, MaxSafeXP} {
		v := LevelFor(xp)
		if v.Threshold > xp || v.NextThreshold <= xp || v.ToNext != v.NextThreshold-xp {
			t.Fatal(v)
		}
	}
}
func TestEconomyV1AndPartialQuotas(t *testing.T) {
	want := []int64{0, 5, 10, 25, 50, 100, 0}
	for i, xp := range want {
		if TaskXP(i) != xp {
			t.Fatal(i)
		}
	}
	if BaseXP("habit", 0) != 10 || BaseXP("challenge", 0) != 25 || BaseXP("goal", 0) != 0 || Quota("task") != 200 || Quota("habit") != 50 {
		t.Fatal("rules")
	}
	for _, c := range []struct{ base, used, total, cap, want int64 }{{25, 197, 197, 200, 3}, {10, 40, 297, 50, 3}, {100, 200, 200, 200, 0}, {10, 0, 300, 50, 0}, {25, 0, 0, 200, 25}} {
		if got := Limited(c.base, c.used, c.total, c.cap); got != c.want {
			t.Fatal(c, got)
		}
	}
}
