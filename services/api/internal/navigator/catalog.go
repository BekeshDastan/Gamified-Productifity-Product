package navigator

// DefaultRules mirrors the immutable version-one seed for deterministic tests.
func DefaultRules() []Rule {
	return []Rule{
		{ID: "R01", Version: 1, Priority: 75, Conditions: []Condition{{Metric: "postponements", Op: "gte", Value: 3}, {Metric: "window_days", Op: "eq", Value: 14}}, TemplateKey: "R01", ActionType: "task_steps", CooldownSeconds: 604800, Enabled: true},
		{ID: "R02", Version: 1, Priority: 90, Conditions: []Condition{{Metric: "deficit_minutes", Op: "gt", Value: 0}}, TemplateKey: "R02", ActionType: "plan_review", CooldownSeconds: 86400, Enabled: true},
		{ID: "R03", Version: 1, Priority: 100, Conditions: []Condition{{Metric: "hours_to_deadline", Op: "gt", Value: 0}, {Metric: "hours_to_deadline", Op: "lt", Value: 24}, {Metric: "deficit_minutes", Op: "gt", Value: 0}}, TemplateKey: "R03", ActionType: "deadline_radar", CooldownSeconds: 86400, Enabled: true},
		{ID: "R04", Version: 1, Priority: 65, Conditions: []Condition{{Metric: "missed_periods", Op: "gte", Value: 2}}, TemplateKey: "R04", ActionType: "habit_review", CooldownSeconds: 604800, Enabled: true},
		{ID: "R05", Version: 1, Priority: 70, Conditions: []Condition{{Metric: "inactive_days", Op: "gte", Value: 3}, {Metric: "returned", Op: "eq", Value: 1}}, TemplateKey: "R05", ActionType: "comeback", CooldownSeconds: 1209600, Enabled: true},
		{ID: "R06", Version: 1, Priority: 85, Conditions: []Condition{{Metric: "low_energy", Op: "eq", Value: 1}}, TemplateKey: "R06", ActionType: "low_energy", CooldownSeconds: 86400, Enabled: true},
		{ID: "R07", Version: 1, Priority: 60, Conditions: []Condition{{Metric: "visible_seconds", Op: "gt", Value: 3600}, {Metric: "first_steps", Op: "eq", Value: 0}}, TemplateKey: "R07", ActionType: "five_minute_start", CooldownSeconds: 86400, Enabled: true},
		{ID: "R08", Version: 1, Priority: 55, Conditions: []Condition{{Metric: "review_missing", Op: "eq", Value: 1}}, TemplateKey: "R08", ActionType: "weekly_reset", CooldownSeconds: 604800, Enabled: true},
		{ID: "R09", Version: 1, Priority: 50, Conditions: []Condition{{Metric: "samples", Op: "gte", Value: 5}, {Metric: "actual_estimate_ratio", Op: "gt", Value: 1.5}}, TemplateKey: "R09", ActionType: "template_estimate", CooldownSeconds: 604800, Enabled: true},
		{ID: "R10", Version: 1, Priority: 45, Conditions: []Condition{{Metric: "next_actions", Op: "eq", Value: 0}}, TemplateKey: "R10", ActionType: "goal_first_action", CooldownSeconds: 604800, Enabled: true},
		{ID: "R11", Version: 1, Priority: 40, Conditions: []Condition{{Metric: "streak", Op: "gte", Value: 7}}, TemplateKey: "R11", ActionType: "achievements", CooldownSeconds: 604800, Enabled: true},
		{ID: "R12", Version: 1, Priority: 95, Conditions: []Condition{{Metric: "overlap_minutes", Op: "gt", Value: 0}}, TemplateKey: "R12", ActionType: "calendar_conflict", CooldownSeconds: 86400, Enabled: true},
		{ID: "R13", Version: 1, Priority: 80, Conditions: []Condition{{Metric: "overdue", Op: "eq", Value: 1}, {Metric: "progress", Op: "gt", Value: 0}}, TemplateKey: "R13", ActionType: "goal_review", CooldownSeconds: 604800, Enabled: true},
		{ID: "R14", Version: 1, Priority: 30, Conditions: []Condition{{Metric: "boss_missing", Op: "eq", Value: 1}}, TemplateKey: "R14", ActionType: "monthly_boss", CooldownSeconds: 604800, Enabled: true},
		{ID: "R15", Version: 1, Priority: 88, Conditions: []Condition{{Metric: "pending_sessions", Op: "gt", Value: 0}}, TemplateKey: "R15", ActionType: "focus_confirmation", CooldownSeconds: 86400, Enabled: true},
	}
}
