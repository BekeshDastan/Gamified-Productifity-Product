// Package navigator evaluates structured, versioned facts without executing stored code.
package navigator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
)

type Condition struct {
	Metric string  `json:"metric"`
	Op     string  `json:"op"`
	Value  float64 `json:"value"`
}
type Rule struct {
	ID              string      `json:"id"`
	Version         int64       `json:"version"`
	Priority        int         `json:"priority"`
	Conditions      []Condition `json:"conditions"`
	TemplateKey     string      `json:"template_key"`
	ActionType      string      `json:"action_type"`
	CooldownSeconds int         `json:"cooldown_seconds"`
	Enabled         bool        `json:"enabled"`
}
type Evidence struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Version int64  `json:"version"`
	URL     string `json:"url"`
}
type Fact struct {
	RuleID  string             `json:"rule_id"`
	Subject string             `json:"subject"`
	Numbers map[string]float64 `json:"numbers"`
	Sources []Evidence         `json:"sources"`
}
type Candidate struct {
	Rule Rule `json:"rule"`
	Fact Fact `json:"fact"`
}

var metrics = map[string][]string{
	"R01": {"postponements", "window_days"}, "R02": {"planned_minutes", "available_minutes", "deficit_minutes"},
	"R03": {"hours_to_deadline", "estimated_minutes", "free_minutes", "deficit_minutes"},
	"R04": {"missed_periods"}, "R05": {"inactive_days", "returned"}, "R06": {"low_energy"},
	"R07": {"visible_seconds", "first_steps"}, "R08": {"review_missing"}, "R09": {"samples", "actual_estimate_ratio"},
	"R10": {"next_actions"}, "R11": {"streak"}, "R12": {"overlap_minutes"}, "R13": {"overdue", "progress"},
	"R14": {"boss_missing"}, "R15": {"pending_sessions"},
}

func DecodeRule(raw []byte) (Rule, error) {
	var r Rule
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(&r); e != nil {
		return r, e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return r, fmt.Errorf("one rule object required")
	}
	return r, ValidateRule(r)
}
func ValidateRule(r Rule) error {
	allowed, ok := metrics[r.ID]
	if !ok || r.Version < 1 || r.Priority < 1 || r.Priority > 100 || r.CooldownSeconds < 3600 || r.CooldownSeconds > 30*86400 || len(r.Conditions) < 1 || len(r.Conditions) > 5 || r.TemplateKey != r.ID || r.ActionType != actions[r.ID] {
		return fmt.Errorf("invalid rule metadata")
	}
	for _, c := range r.Conditions {
		valid := false
		for _, m := range allowed {
			valid = valid || c.Metric == m
		}
		if !valid || math.IsNaN(c.Value) || math.IsInf(c.Value, 0) || (c.Op != "gt" && c.Op != "gte" && c.Op != "eq" && c.Op != "lt") {
			return fmt.Errorf("invalid typed condition")
		}
	}
	return nil
}

var actions = map[string]string{"R01": "task_steps", "R02": "plan_review", "R03": "deadline_radar", "R04": "habit_review", "R05": "comeback", "R06": "low_energy", "R07": "five_minute_start", "R08": "weekly_reset", "R09": "template_estimate", "R10": "goal_first_action", "R11": "achievements", "R12": "calendar_conflict", "R13": "goal_review", "R14": "monthly_boss", "R15": "focus_confirmation"}

func Evaluate(rules []Rule, facts []Fact) ([]Candidate, error) {
	out := []Candidate{}
	for _, r := range rules {
		if e := ValidateRule(r); e != nil {
			return nil, e
		}
		if !r.Enabled {
			continue
		}
		for _, f := range facts {
			if f.RuleID != r.ID {
				continue
			}
			matches := true
			for _, c := range r.Conditions {
				v, ok := f.Numbers[c.Metric]
				matches = matches && ok && !math.IsNaN(v) && !math.IsInf(v, 0) && ((c.Op == "gt" && v > c.Value) || (c.Op == "gte" && v >= c.Value) || (c.Op == "eq" && v == c.Value) || (c.Op == "lt" && v < c.Value))
			}
			if matches {
				out = append(out, Candidate{r, f})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Rule.Priority != b.Rule.Priority {
			return a.Rule.Priority > b.Rule.Priority
		}
		if a.Rule.ID != b.Rule.ID {
			return a.Rule.ID < b.Rule.ID
		}
		return a.Fact.Subject < b.Fact.Subject
	})
	return out, nil
}
