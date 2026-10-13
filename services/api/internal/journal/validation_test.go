package journal

import (
	"strings"
	"testing"
)

func TestJournalValidationBounds(t *testing.T) {
	valid := Input{Date: "2026-10-12", Title: "Заголовок", Body: "Текст"}
	for _, mutate := range []func(*Input){func(v *Input) { v.Date = "bad" }, func(v *Input) { v.Title = "" }, func(v *Input) { v.Title = strings.Repeat("я", 201) }, func(v *Input) { v.Title = "a\nb" }, func(v *Input) { v.Body = strings.Repeat("я", 50001) }, func(v *Input) { v.Body = "a\x00b" }, func(v *Input) { s := "bad"; v.GoalID = &s }, func(v *Input) { v.Tags = make([]string, 21) }, func(v *Input) { v.Tags = []string{""} }, func(v *Input) { v.Tags = []string{"a\nb"} }, func(v *Input) { v.Tags = []string{strings.Repeat("x", 51)} }, func(v *Input) { v.Tags = []string{"same", "same"} }} {
		v := valid
		mutate(&v)
		if _, e := validate(v); e == nil {
			t.Fatal(v)
		}
	}
	if v, e := validate(valid); e != nil || v.Tags == nil {
		t.Fatal(v, e)
	}
	for _, in := range []ReviewInput{{Obstacle: "bad", Conclusion: "x"}, {Obstacle: "none"}, {Obstacle: "none", Conclusion: strings.Repeat("я", 5001)}, {Obstacle: "none", Conclusion: "x", ObstacleNote: strings.Repeat("я", 2001)}, {Obstacle: "none", Conclusion: "x\x00"}} {
		if _, e := validateReview(in); e == nil {
			t.Fatal(in)
		}
	}
	zero, six := 0, 6
	for _, in := range []ReviewInput{{Obstacle: "none", Conclusion: "x", Energy: &zero}, {Obstacle: "none", Conclusion: "x", Mood: &six}} {
		if _, e := validateReview(in); e == nil {
			t.Fatal(in)
		}
	}
	if _, e := validateReview(ReviewInput{Obstacle: "low_energy", Conclusion: "Достаточно одного шага"}); e != nil {
		t.Fatal(e)
	}
	for _, in := range []TemplateInput{{Steps: make([]ChecklistStep, 21)}, {Steps: []ChecklistStep{{ID: "!", Label: "x"}}}, {Steps: []ChecklistStep{{ID: "a", Label: ""}}}, {Steps: []ChecklistStep{{ID: "a", Label: "x"}, {ID: "a", Label: "x"}}}} {
		if _, e := validateTemplate(in); e == nil {
			t.Fatal(in)
		}
	}
	if out, e := validateTemplate(TemplateInput{}); e != nil || out.Steps == nil {
		t.Fatal(out, e)
	}
	if templateKind("invalid") == nil || defaultTemplate("evening").Steps[0].ID != "outcomes" {
		t.Fatal("kind")
	}
}
