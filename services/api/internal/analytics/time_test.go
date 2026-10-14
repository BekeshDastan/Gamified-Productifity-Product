package analytics

import "testing"

func TestTemplateSamplesUseKnownPairsAndKeepVersionsSeparate(t *testing.T) {
	id, name, version := "recorded-template", "Практика", int64(1)
	estimate, actual := 10, int64(1200)
	records := []TaskTime{{ID: "unrelated", EstimatedMinutes: &estimate, ActualSeconds: &actual}, {ID: "unknown", TemplateID: &id, TemplateName: &name, TemplateVersion: &version}}
	for _, task := range []string{"e", "d", "c", "b", "a"} {
		records = append(records, TaskTime{ID: task, TemplateID: &id, TemplateName: &name, TemplateVersion: &version, EstimatedMinutes: &estimate, ActualSeconds: &actual})
	}
	version2 := int64(2)
	records = append(records, TaskTime{ID: "v2", TemplateID: &id, TemplateName: &name, TemplateVersion: &version2, EstimatedMinutes: &estimate, ActualSeconds: &actual})
	samples := templateSamples(records)
	if len(samples) != 2 || samples[0].Count != 5 || !samples[0].Sufficient || *samples[0].Factor != 2 || samples[0].QuestIDs[0] != "a" || samples[1].Sufficient || samples[1].Count != 1 {
		t.Fatal(samples)
	}
	onlyUnknown := templateSamples(records[1:2])
	if len(onlyUnknown) != 1 || onlyUnknown[0].Factor != nil || onlyUnknown[0].AverageActualSeconds != nil || onlyUnknown[0].Sufficient {
		t.Fatal("unknown became zero", onlyUnknown)
	}
	zero := 0
	records = []TaskTime{{ID: "zero", TemplateID: &id, TemplateName: &name, TemplateVersion: &version, EstimatedMinutes: &zero, ActualSeconds: &actual}}
	if v := templateSamples(records); v[0].Factor != nil {
		t.Fatal("division by zero", v)
	}
}
