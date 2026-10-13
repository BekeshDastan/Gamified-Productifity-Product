package character

import "testing"

func TestSkillDAGRejectsCyclesAndExplainsEveryNode(t *testing.T) {
	if e := ValidateSkills(SkillCatalog()); e != nil {
		t.Fatal(e)
	}
	nodes := SkillCatalog()
	nodes[0].DependsOn = []string{"depth"}
	if ValidateSkills(nodes) == nil {
		t.Fatal("cycle allowed")
	}
	nodes = SkillCatalog()
	nodes[0].DependsOn = []string{"missing"}
	if ValidateSkills(nodes) == nil {
		t.Fatal("missing dependency")
	}
	for _, s := range skillsFor(Area{XP: 400, SuccessfulPeriods: 7}) {
		if !s.Unlocked || s.Explanation == "" {
			t.Fatal(s)
		}
	}
	for _, s := range skillsFor(Area{XP: 400}) {
		if s.ID == "depth" && s.Unlocked {
			t.Fatal("unmet dependency")
		}
	}
}
