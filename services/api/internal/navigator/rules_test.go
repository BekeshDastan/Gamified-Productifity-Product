package navigator

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRulesDeterministicTypedAndComplete(t *testing.T) {
	rules := DefaultRules()
	facts := []Fact{}
	for _, r := range rules {
		if e := ValidateRule(r); e != nil {
			t.Fatal(r.ID, e)
		}
		numbers := map[string]float64{}
		for _, c := range r.Conditions {
			numbers[c.Metric] = c.Value
			if c.Op == "gt" {
				numbers[c.Metric]++
			}
			if c.Op == "lt" {
				numbers[c.Metric]--
			}
		}
		facts = append(facts, Fact{r.ID, r.ID, numbers, []Evidence{}})
		raw, _ := json.Marshal(r)
		decoded, e := DecodeRule(raw)
		if e != nil || !reflect.DeepEqual(decoded, r) {
			t.Fatal(decoded, e)
		}
	}
	a, e := Evaluate(rules, facts)
	if e != nil || len(a) != 15 {
		t.Fatal(a, e)
	}
	b, e := Evaluate(rules, facts)
	if e != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("unstable", e)
	}
	for _, f := range facts {
		for k := range f.Numbers {
			f.Numbers[k] = -1
		}
	}
	if out, e := Evaluate(rules, facts); e != nil || len(out) != 0 {
		t.Fatal(out, e)
	}
}
func TestRuleRejectsCodeUnknownMetricsAndInvalidMetadata(t *testing.T) {
	r := DefaultRules()[0]
	raw, _ := json.Marshal(r)
	malicious := append([]byte(`{"code":"fetch('https://example.test')",`), raw[1:]...)
	if _, e := DecodeRule(malicious); e == nil {
		t.Fatal("executable payload accepted")
	}
	if _, e := DecodeRule(append(raw, []byte(` {}`)...)); e == nil {
		t.Fatal("trailing payload accepted")
	}
	r.Conditions = []Condition{{"diary", "gt", 0}}
	if ValidateRule(r) == nil {
		t.Fatal("private text condition accepted")
	}
	r = DefaultRules()[0]
	r.ActionType = "javascript"
	if ValidateRule(r) == nil {
		t.Fatal("unknown action")
	}
	r = DefaultRules()[0]
	r.CooldownSeconds = 0
	if ValidateRule(r) == nil {
		t.Fatal("unbounded cooldown")
	}
}
