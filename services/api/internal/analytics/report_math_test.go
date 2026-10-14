package analytics

import (
	"strings"
	"testing"
)

func TestReportsCompareEqualPeriodsAndZeroBase(t *testing.T) {
	current, previous, e := reportRanges(ReportInput{Kind: "month", StartsOn: "2024-02-01"})
	if e != nil || current.To != "2024-03-01" || previous.From != "2024-01-03" || previous.To != "2024-02-01" {
		t.Fatal(current, previous, e)
	}
	for _, in := range []ReportInput{{Kind: "week", StartsOn: "2026-10-13"}, {Kind: "month", StartsOn: "2026-10-02"}, {Kind: "other", StartsOn: "2026-10-12"}, {Kind: "week", StartsOn: "bad"}} {
		if _, _, e := reportRanges(in); e == nil {
			t.Fatal("invalid report period", in)
		}
	}
	v := compare("tasks", "tasks", 1, 0, true, true)
	if v.Previous == nil || v.Delta == nil || *v.Delta != 1 || v.Percent != nil {
		t.Fatal("infinite growth", v)
	}
	v = compare("tasks", "tasks", 3, 2, true, true)
	if v.Percent == nil || *v.Percent != 50 {
		t.Fatal(v)
	}
	v = compare("tasks", "tasks", 1, 0, true, false)
	if v.Previous != nil || v.Delta != nil || v.Percent != nil {
		t.Fatal("missing observations replaced by zero", v)
	}
	text := reportText("month", current, previous, []Comparison{v}, false, false)
	if !strings.Contains(text, "Наблюдения за периодом неполные") || !strings.Contains(text, "недостаточно наблюдений") {
		t.Fatal(text)
	}
}
