package planning

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlanValidation(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	for _, in := range []Input{{AvailableMinutes: -1}, {AvailableMinutes: 1441}, {Windows: make([]Window, 9)}, {QuestIDs: []string{"bad"}}, {QuestIDs: []string{id, id}}, {TopIDs: []string{id}}, {QuestIDs: []string{id}, TopIDs: []string{id, id}}, {Rest: true, AvailableMinutes: 1}, {Windows: []Window{{"12:00", "11:00"}}}, {Windows: []Window{{"24:00", "24:00"}}}, {Windows: []Window{{"09:00", "25:00"}}}} {
		if _, e := Validate(in); e == nil {
			t.Fatal("invalid input accepted", in)
		}
	}
	if v, e := Validate(Input{Windows: []Window{{"00:00", "24:00"}}, QuestIDs: []string{id}, TopIDs: []string{id}}); e != nil || v.TopIDs[0] != id {
		t.Fatal(v, e)
	}
	for _, d := range []string{"2026-02-29", "0000-01-01", "9999-01-01", "2026-1-01"} {
		if _, e := day(d); e == nil {
			t.Fatal(d)
		}
	}
	for _, s := range []string{"", strings.Repeat("a", 1001), "a\x00b"} {
		if reason(s) == nil {
			t.Fatal("invalid reason")
		}
	}
	for _, tc := range []struct {
		header  string
		current int64
		valid   bool
	}{{`"0"`, 0, true}, {`"0"`, 1, false}, {`"1"`, 0, false}, {`"1"`, 1, true}, {`"1"`, 2, false}, {"", 1, false}} {
		r := httptest.NewRequest("PUT", "/", nil)
		r.Header.Set("If-Match", tc.header)
		if (matchVersion(r, tc.current) == nil) != tc.valid {
			t.Fatal(tc)
		}
	}
	if _, _, e := bounds("bad", "UTC"); e == nil {
		t.Fatal("bad date")
	}
	if _, _, e := bounds("2026-10-12", "bad"); e == nil {
		t.Fatal("bad zone")
	}
	for _, tc := range [][3]string{{"bad", "UTC", "09:00"}, {"2026-10-12", "bad", "09:00"}, {"2026-10-12", "UTC", "99:00"}} {
		if _, e := wall(tc[0], tc[1], tc[2], false); e == nil {
			t.Fatal(tc)
		}
	}
}
func TestRoutineValidation(t *testing.T) {
	for _, b := range builtins() {
		if _, e := validateRoutine(b.RoutineInput); e != nil {
			t.Fatal(e)
		}
	}
	base := func() RoutineInput {
		return RoutineInput{Title: "Шаблон", Period: "day", Days: []RoutineDay{{Day: 1, AvailableMinutes: 60, Actions: []RoutineAction{{Key: "a", Title: "Шаг", Priority: "normal", Minutes: 30}}}}}
	}
	mutations := []func(*RoutineInput){func(v *RoutineInput) { v.Title = "" }, func(v *RoutineInput) { v.Title = "a\x00b" }, func(v *RoutineInput) { v.Period = "year" }, func(v *RoutineInput) { v.Days = nil }, func(v *RoutineInput) { v.Days[0].Day = 2 }, func(v *RoutineInput) { v.Days[0].AvailableMinutes = -1 }, func(v *RoutineInput) { v.Days[0].Rest = true; v.Days[0].AvailableMinutes = 0 }, func(v *RoutineInput) { v.Days[0].Actions[0].Key = "" }, func(v *RoutineInput) { v.Days[0].Actions[0].Priority = "bad" }, func(v *RoutineInput) { v.Days[0].Actions[0].Start = "09:00" }, func(v *RoutineInput) { v.Days[0].Actions[0].Start = "bad"; v.Days[0].Actions[0].End = "12:00" }, func(v *RoutineInput) { v.Days[0].Actions[0].Start = "09:00"; v.Days[0].Actions[0].End = "bad" }, func(v *RoutineInput) { v.Days[0].Actions[0].Start = "12:00"; v.Days[0].Actions[0].End = "09:00" }}
	for _, change := range mutations {
		v := base()
		change(&v)
		if _, e := validateRoutine(v); e == nil {
			t.Fatal("invalid routine accepted", v)
		}
	}
}
