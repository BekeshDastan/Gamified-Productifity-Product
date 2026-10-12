package profile

import (
	"testing"
	"time"
)

func TestPreferencesValidationAndTimezoneDates(t *testing.T) {
	base := Preferences{Timezone: "UTC", Language: "ru", WeekStart: 1, TimeFormat: "24h", WorkIntervals: []WorkInterval{}, RestDays: []int{6, 7}}
	for zone, date := range map[string]string{"UTC": "2026-10-12", "Asia/Almaty": "2026-10-13", "America/New_York": "2026-10-12"} {
		p := base
		p.Timezone = zone
		if err := ValidatePreferences(p); err != nil {
			t.Fatal(err)
		}
		location, _ := time.LoadLocation(zone)
		if got := time.Date(2026, 10, 12, 22, 30, 0, 0, time.UTC).In(location).Format(time.DateOnly); got != date {
			t.Fatal("wrong local date", zone)
		}
	}
	for _, zone := range []string{"Local", "Invalid/Zone", "+05:00", "../UTC"} {
		p := base
		p.Timezone = zone
		if ValidatePreferences(p) == nil {
			t.Fatal("invalid timezone accepted")
		}
	}
	for _, intervals := range [][]WorkInterval{{{Day: 6, Start: "09:00", End: "10:00"}}, {{Day: 1, Start: "22:00", End: "09:00"}}, {{Day: 1, Start: "09:00", End: "12:00"}, {Day: 1, Start: "11:00", End: "13:00"}}} {
		p := base
		p.WorkIntervals = intervals
		if ValidatePreferences(p) == nil {
			t.Fatal("invalid schedule accepted")
		}
	}
	p := base
	p.WorkIntervals = []WorkInterval{{Day: 1, Start: "09:00", End: "12:00"}, {Day: 1, Start: "12:00", End: "13:00"}}
	if err := ValidatePreferences(p); err != nil {
		t.Fatal(err)
	}
}
