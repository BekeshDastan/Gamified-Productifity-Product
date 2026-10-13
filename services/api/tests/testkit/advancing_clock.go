package testkit

import "time"

// AdvancingClock lets real browser tests cross focus boundaries without waiting
// five wall-clock minutes. Only the isolated browser fixture uses this clock.
type AdvancingClock struct {
	base    time.Time
	started time.Time
	rate    time.Duration
}

func NewAdvancingClock(base time.Time, rate int) *AdvancingClock {
	if rate < 1 {
		rate = 1
	}
	return &AdvancingClock{base.UTC(), time.Now(), time.Duration(rate)}
}
func (c *AdvancingClock) Now() time.Time { return c.base.Add(time.Since(c.started) * c.rate) }
