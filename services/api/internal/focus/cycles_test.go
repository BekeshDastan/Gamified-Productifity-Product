package focus

import "testing"

func TestSettingsBounds(t *testing.T) {
	for _, mode := range []string{"free", "pomodoro", "quick"} {
		in := StartInput{OperationID: "11111111-1111-4111-8111-111111111111", Mode: mode}
		out, e := validateStart(in)
		if e != nil {
			t.Fatal(mode, e)
		}
		if mode == "quick" && out.WorkMinutes != 5 {
			t.Fatal(out)
		}
	}
	for _, in := range []StartInput{{Mode: "pomodoro", WorkMinutes: 4, BreakMinutes: 5}, {Mode: "pomodoro", WorkMinutes: 121, BreakMinutes: 5}, {Mode: "pomodoro", WorkMinutes: 25, BreakMinutes: 61}, {Mode: "pomodoro", WorkMinutes: -1}, {Mode: "quick", WorkMinutes: 10}, {Mode: "unknown"}} {
		in.OperationID = "11111111-1111-4111-8111-111111111111"
		if _, e := validateStart(in); e == nil {
			t.Fatal("accepted", in)
		}
	}
}
