package platform

import "time"

type Clock interface{ Now() time.Time }
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

type FixedClock struct{ Time time.Time }

func (f FixedClock) Now() time.Time { return f.Time }
