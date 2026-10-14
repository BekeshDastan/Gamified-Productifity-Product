package notifications

import (
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"testing"
	"time"
)

func TestQuietCrossMidnightTimezoneAndDST(t *testing.T) {
	for _, v := range []struct{ now, zone, start, end, want string }{{"2026-10-12T23:15:00Z", "UTC", "22:00", "08:00", "2026-10-13T08:00:00Z"}, {"2026-10-12T06:00:00Z", "UTC", "22:00", "08:00", "2026-10-12T08:00:00Z"}, {"2026-10-12T08:00:00Z", "UTC", "22:00", "08:00", "2026-10-12T08:00:00Z"}, {"2026-10-12T18:00:00Z", "Asia/Almaty", "22:00", "08:00", "2026-10-13T03:00:00Z"}, {"2026-03-08T06:45:00Z", "America/New_York", "01:00", "02:30", "2026-03-08T07:00:00Z"}, {"2026-11-01T06:15:00Z", "America/New_York", "00:00", "01:30", "2026-11-01T06:30:00Z"}, {"2026-10-12T13:00:00Z", "UTC", "12:00", "14:00", "2026-10-12T14:00:00Z"}} {
		now, _ := time.Parse(time.RFC3339, v.now)
		got, e := QuietUntil(now, v.zone, QuietWindow{true, v.start, v.end}, false)
		if e != nil || got.Format(time.RFC3339) != v.want {
			t.Fatal(v, got, e)
		}
		urgent, e := QuietUntil(now, v.zone, QuietWindow{true, v.start, v.end}, true)
		if e != nil || !urgent.Equal(now) {
			t.Fatal("urgent not explicit", urgent, e)
		}
	}
}
func TestDeliveryCapsAndExpiredQuietDeferral(t *testing.T) {
	now := time.Date(2026, 10, 12, 23, 0, 0, 0, time.UTC)
	p := Preferences{PreferencesInput: DefaultPreferences(), EffectiveTimezone: "UTC"}
	at, ok, e := PolicyTime(now, now.Add(time.Hour), p, false, 0, nil)
	if e != nil || ok || at.Hour() != 8 {
		t.Fatal(at, ok, e)
	}
	oldest := now.Add(-time.Hour)
	at, ok, e = PolicyTime(now, time.Time{}, p, true, p.DailyCap, &oldest)
	if e != nil || !ok || !at.Equal(oldest.Add(24*time.Hour)) {
		t.Fatal(at, ok, e)
	}
	p.DailyCap = 0
	if _, ok, e = PolicyTime(now, time.Time{}, p, true, 0, nil); e != nil || ok {
		t.Fatal("disabled cap ignored")
	}
}
func TestPreferenceSchemaRejectsDuplicatesAndUnknownTimezone(t *testing.T) {
	p := DefaultPreferences()
	if e := ValidatePreferences(p); e != nil {
		t.Fatal(e)
	}
	p.Channels[1].Type = p.Channels[0].Type
	if ValidatePreferences(p) == nil {
		t.Fatal("duplicate")
	}
	p = DefaultPreferences()
	p.Timezone = "Local"
	if ValidatePreferences(p) == nil {
		t.Fatal("local timezone")
	}
	p = DefaultPreferences()
	p.Quiet.End = p.Quiet.Start
	if ValidatePreferences(p) == nil {
		t.Fatal("ambiguous full-day quiet")
	}
}
func TestPreferenceBoundsAndInvalidQuietTimes(t *testing.T) {
	for _, change := range []func(*PreferencesInput){func(p *PreferencesInput) { p.Timezone = "Missing/Zone" }, func(p *PreferencesInput) { p.Quiet.Start = "25:00" }, func(p *PreferencesInput) { p.Quiet.End = "7:00" }, func(p *PreferencesInput) { p.DailyCap = 51 }, func(p *PreferencesInput) { p.DeadlineLeadMinutes = -1 }, func(p *PreferencesInput) { p.HabitLeadMinutes = 1441 }, func(p *PreferencesInput) { p.Channels = p.Channels[:1] }, func(p *PreferencesInput) { p.Channels[0].Type = "unknown" }} {
		p := DefaultPreferences()
		change(&p)
		if ValidatePreferences(p) == nil {
			t.Fatal("invalid preference accepted")
		}
	}
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	for _, v := range []QuietWindow{{true, "25:00", "08:00"}, {true, "22:00", "25:00"}, {true, "08:00", "08:00"}} {
		if _, e := QuietUntil(now, "UTC", v, false); e == nil {
			t.Fatal("invalid quiet window accepted")
		}
	}
	if _, e := QuietUntil(now, "Missing/Zone", QuietWindow{true, "22:00", "08:00"}, false); e == nil {
		t.Fatal("unknown quiet zone")
	}
	p := Preferences{PreferencesInput: DefaultPreferences(), EffectiveTimezone: "UTC"}
	p.Quiet.Enabled = false
	if p.Channel("unknown").Email {
		t.Fatal("unknown channel enabled")
	}
	at, ok, e := PolicyTime(now, time.Time{}, p, false, p.DailyCap, nil)
	if e != nil || ok || !at.Equal(now) {
		t.Fatal("cap without oldest must fail closed", at, ok, e)
	}
}
func TestNotificationSchedulingClockPreservesDelay(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	worker := now.AddDate(0, 0, 2)
	s := NewService(nil, platform.FixedClock{Time: now}).WithSchedulingClock(platform.FixedClock{Time: worker})
	for _, delay := range []time.Duration{-time.Hour, 0, time.Hour} {
		at, created := s.jobTimes(now.Add(delay))
		if !created.Equal(worker) || !at.Equal(worker.Add(max(0, delay))) {
			t.Fatal(at, created)
		}
	}
}
