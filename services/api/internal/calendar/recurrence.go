package calendar

import (
	"time"
)

type Rule struct {
	Frequency string  `json:"frequency"`
	Interval  int     `json:"interval"`
	Weekdays  []int   `json:"weekdays"`
	Until     *string `json:"until"`
	Count     int     `json:"count"`
}
type SeriesInput struct {
	Event Input  `json:"event"`
	Rule  Rule   `json:"rule"`
	Kind  string `json:"kind"`
}
type Series struct {
	SeriesInput
	ID         string  `json:"id"`
	Version    int64   `json:"version"`
	EndsBefore *string `json:"ends_before"`
}
type Occurrence struct {
	Key   string
	Event Input
}

func validateSeries(in SeriesInput) (SeriesInput, error) {
	v, e := Validate(in.Event)
	if e != nil {
		return in, e
	}
	in.Event = v
	if in.Rule.Weekdays == nil {
		in.Rule.Weekdays = []int{}
	}
	if !v.AllDay {
		start, _ := time.Parse(time.RFC3339, v.Start)
		if start.Second() != 0 || start.Nanosecond() != 0 {
			return in, invalid("start", "Для повторений укажите время с точностью до минуты")
		}
	}
	if in.Event.QuestID != nil {
		return in, invalid("quest_id", "Для повторения задачи создаётся отдельная задача на каждый экземпляр")
	}
	if in.Kind != "event" && in.Kind != "task" {
		return in, invalid("kind", "Выберите событие или отдельные повторяющиеся задачи")
	}
	if in.Rule.Frequency != "daily" && in.Rule.Frequency != "weekly" && in.Rule.Frequency != "monthly" {
		return in, invalid("frequency", "Выберите ежедневное, недельное или месячное повторение")
	}
	if in.Rule.Interval < 1 || in.Rule.Interval > 52 {
		return in, invalid("interval", "Интервал: 1–52")
	}
	if len(in.Rule.Weekdays) > 7 {
		return in, invalid("weekdays", "Выберите дни недели")
	}
	seen := map[int]bool{}
	for _, d := range in.Rule.Weekdays {
		if d < 1 || d > 7 || seen[d] {
			return in, invalid("weekdays", "Дни недели: 1–7 без повторений")
		}
		seen[d] = true
	}
	if in.Rule.Frequency == "weekly" && len(seen) == 0 {
		return in, invalid("weekdays", "Выберите хотя бы один день недели")
	}
	if in.Rule.Frequency != "weekly" && len(seen) > 0 {
		return in, invalid("weekdays", "Дни недели относятся к недельному повторению")
	}
	anchor, _, _ := seriesAnchor(in.Event)
	if (in.Rule.Until == nil) == (in.Rule.Count == 0) || in.Rule.Count < 0 || in.Rule.Count > 10000 {
		return in, invalid("count", "Выберите конец по дате или числу 1–10000")
	}
	if in.Rule.Until != nil {
		until, e := day(*in.Rule.Until)
		if e != nil || until.Before(anchor) || until.After(anchor.AddDate(10, 0, 0)) {
			return in, invalid("until", "Конец серии: от начала до 10 лет")
		}
	}
	if in.Rule.Count > 0 {
		all := generateValidated(in, anchor, anchor.AddDate(10, 0, 1), nil)
		if len(all) < in.Rule.Count {
			return in, invalid("count", "Выбранное число повторений не помещается в 10 лет; уменьшите число или интервал")
		}
	}
	return in, nil
}
func seriesAnchor(in Input) (time.Time, int, int) {
	if in.AllDay {
		d, _ := day(in.Start)
		return d, 0, 0
	}
	zone, _ := time.LoadLocation(in.Timezone)
	d, _ := time.Parse(time.RFC3339, in.Start)
	d = d.In(zone)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC), d.Hour(), d.Minute()
}

// ResolveWall chooses the earlier instant for a fold; a missing wall time is skipped.
// Testing offsets around the wall date also covers non-hour DST transitions.
func ResolveWall(d time.Time, hour, minute int, zone *time.Location) (time.Time, bool) {
	wall := time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, time.UTC)
	offsets := map[int]bool{}
	for h := -48; h <= 48; h += 6 {
		_, off := wall.Add(time.Duration(h) * time.Hour).In(zone).Zone()
		offsets[off] = true
	}
	var earliest time.Time
	for off := range offsets {
		candidate := wall.Add(-time.Duration(off) * time.Second)
		local := candidate.In(zone)
		if local.Year() == d.Year() && local.Month() == d.Month() && local.Day() == d.Day() && local.Hour() == hour && local.Minute() == minute && (earliest.IsZero() || candidate.Before(earliest)) {
			earliest = candidate
		}
	}
	return earliest, !earliest.IsZero()
}
func Generate(in SeriesInput, from, to time.Time, endsBefore *string) ([]Occurrence, error) {
	valid, e := validateSeries(in)
	if e != nil {
		return nil, e
	}
	in = valid
	return generateValidated(in, from, to, endsBefore), nil
}
func generateValidated(in SeriesInput, from, to time.Time, endsBefore *string) []Occurrence {
	anchor, h, m := seriesAnchor(in.Event)
	zone, _ := time.LoadLocation(in.Event.Timezone)
	var duration time.Duration
	days := 0
	if in.Event.AllDay {
		end, _ := day(in.Event.End)
		days = int(end.Sub(anchor) / (24 * time.Hour))
	} else {
		start, _ := time.Parse(time.RFC3339, in.Event.Start)
		end, _ := time.Parse(time.RFC3339, in.Event.End)
		duration = end.Sub(start)
	}
	limit := anchor.AddDate(10, 0, 1)
	if in.Rule.Until != nil {
		d, _ := day(*in.Rule.Until)
		limit = d.AddDate(0, 0, 1)
	}
	if endsBefore != nil {
		d, _ := day(*endsBefore)
		if d.Before(limit) {
			limit = d
		}
	}
	out := []Occurrence{}
	number := 0
	weekday := func(d time.Time) int { return (int(d.Weekday())+6)%7 + 1 }
	weekAnchor := anchor.AddDate(0, 0, 1-weekday(anchor))
	for d := anchor; d.Before(limit) && d.Before(to); d = d.AddDate(0, 0, 1) {
		match := false
		switch in.Rule.Frequency {
		case "daily":
			match = int(d.Sub(anchor)/(24*time.Hour))%in.Rule.Interval == 0
		case "weekly":
			week := int(d.Sub(weekAnchor) / (7 * 24 * time.Hour))
			for _, w := range in.Rule.Weekdays {
				if w == weekday(d) && week%in.Rule.Interval == 0 {
					match = true
				}
			}
		case "monthly":
			months := (d.Year()-anchor.Year())*12 + int(d.Month()-anchor.Month())
			match = d.Day() == anchor.Day() && months%in.Rule.Interval == 0
		}
		if !match {
			continue
		}
		v := in.Event
		key := d.Format("2006-01-02")
		if in.Event.AllDay {
			v.Start = key
			v.End = d.AddDate(0, 0, days).Format("2006-01-02")
		} else {
			start, exists := ResolveWall(d, h, m, zone)
			if !exists {
				continue
			}
			v.Start = start.Format(time.RFC3339)
			v.End = start.Add(duration).Format(time.RFC3339)
		}
		number++
		if in.Rule.Count > 0 && number > in.Rule.Count {
			break
		}
		// Include overlap at the start of the requested window, including long blocks.
		lookback := days
		if !in.Event.AllDay {
			lookback = int(duration/(24*time.Hour)) + 3
		}
		overlap := d.AddDate(0, 0, lookback).After(from)
		if overlap {
			out = append(out, Occurrence{Key: key, Event: v})
		}
	}
	return out
}
