package notifications

import (
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
