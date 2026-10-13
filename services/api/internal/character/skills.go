package character

import (
	"errors"
	"fmt"
)

type SkillDefinition struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Metric    string   `json:"metric"`
	Threshold int64    `json:"threshold"`
	DependsOn []string `json:"depends_on"`
}
type Skill struct {
	SkillDefinition
	Current     int64  `json:"current"`
	Unlocked    bool   `json:"unlocked"`
	Explanation string `json:"explanation"`
}

func SkillCatalog() []SkillDefinition {
	return []SkillDefinition{{"foundation", "Первый вклад", "xp", 5, []string{}}, {"momentum", "Движение", "xp", 100, []string{"foundation"}}, {"regularity", "Семь норм", "periods", 7, []string{"foundation"}}, {"depth", "Устойчивый вклад", "xp", 400, []string{"momentum", "regularity"}}}
}
func ValidateSkills(nodes []SkillDefinition) error {
	defs := map[string]SkillDefinition{}
	for _, n := range nodes {
		if n.ID == "" || n.Name == "" || n.Threshold < 1 || (n.Metric != "xp" && n.Metric != "periods") {
			return errors.New("invalid skill")
		}
		if _, ok := defs[n.ID]; ok {
			return errors.New("duplicate skill")
		}
		defs[n.ID] = n
	}
	states := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if states[id] == 1 {
			return errors.New("skill cycle")
		}
		if states[id] == 2 {
			return nil
		}
		node, ok := defs[id]
		if !ok {
			return errors.New("missing dependency")
		}
		states[id] = 1
		for _, dep := range node.DependsOn {
			if e := visit(dep); e != nil {
				return e
			}
		}
		states[id] = 2
		return nil
	}
	for _, node := range nodes {
		if e := visit(node.ID); e != nil {
			return e
		}
	}
	return nil
}
func skillsFor(a Area) []Skill {
	catalog := SkillCatalog()
	out := []Skill{}
	unlocked := map[string]bool{}
	var evaluate func(string) bool
	evaluate = func(id string) bool {
		if v, ok := unlocked[id]; ok {
			return v
		}
		for _, node := range catalog {
			if node.ID == id {
				current := a.XP
				if node.Metric == "periods" {
					current = a.SuccessfulPeriods
				}
				ok := current >= node.Threshold
				for _, dep := range node.DependsOn {
					ok = evaluate(dep) && ok
				}
				unlocked[id] = ok
				return ok
			}
		}
		return false
	}
	for _, n := range catalog {
		current, unit := a.XP, "XP направления"
		if n.Metric == "periods" {
			current = a.SuccessfulPeriods
			unit = "зачтённых норм привычки"
		}
		description := fmt.Sprintf("Нужно %d %s; сейчас %d. Зависимости: %v", n.Threshold, unit, current, n.DependsOn)
		out = append(out, Skill{n, current, evaluate(n.ID), description})
	}
	return out
}
