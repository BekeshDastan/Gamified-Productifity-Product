package planning

import (
	"testing"
	"time"
)

func TestRadarBoundariesSeparateOverdueAndDay(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	for _, v := range []struct {
		offset time.Duration
		index  int
	}{{-time.Second, 0}, {0, 1}, {24*time.Hour - time.Second, 1}, {24 * time.Hour, 2}, {72 * time.Hour, 2}, {72*time.Hour + time.Second, 3}, {7 * 24 * time.Hour, 3}, {7*24*time.Hour + time.Second, -1}} {
		if got := radarIndex(now.Add(v.offset), now); got != v.index {
			t.Fatal(v, got)
		}
	}
}
