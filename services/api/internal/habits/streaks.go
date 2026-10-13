package habits

import "time"

type Stats struct {
	Current    int  `json:"current"`
	Best       int  `json:"best"`
	Successful int  `json:"successful"`
	Missed     int  `json:"missed"`
	Rate       int  `json:"rate"`
	Recovering bool `json:"recovering"`
}

func CalculateStats(periods []Period, now time.Time) Stats {
	var s Stats
	run := 0
	hadMiss := false
	for _, p := range periods {
		if p.Excluded {
			continue
		}
		if p.Amount >= p.Target {
			run++
			s.Successful++
			s.Best = max(s.Best, run)
		} else if !now.Before(p.End) {
			run = 0
			s.Missed++
			hadMiss = true
		}
	}
	s.Current = run
	s.Recovering = hadMiss && run > 0
	if s.Successful+s.Missed > 0 {
		s.Rate = s.Successful * 100 / (s.Successful + s.Missed)
	}
	return s
}
