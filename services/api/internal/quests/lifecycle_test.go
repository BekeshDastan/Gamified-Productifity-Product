package quests

import "testing"

func TestTransitionMatrix(t *testing.T) {
	allowed := map[string]map[string]string{
		"backlog":     {"ready": "todo", "start": "in_progress", "complete": "completed", "skip": "skipped", "fail": "failed"},
		"todo":        {"start": "in_progress", "complete": "completed", "postpone": "postponed", "skip": "skipped", "fail": "failed"},
		"in_progress": {"ready": "todo", "complete": "completed", "postpone": "postponed", "skip": "skipped", "fail": "failed"},
		"postponed":   {"ready": "todo", "start": "in_progress", "complete": "completed", "postpone": "postponed", "skip": "skipped", "fail": "failed"},
		"completed":   {"reopen": "todo"}, "failed": {"reopen": "todo"}, "skipped": {"reopen": "todo"},
	}
	for state, moves := range allowed {
		for _, action := range []string{"ready", "start", "complete", "reopen", "postpone", "skip", "fail"} {
			got, err := nextStatus(state, action)
			want, ok := moves[action]
			if ok && (err != nil || got != want) {
				t.Fatalf("%s/%s: %q %v", state, action, got, err)
			}
			if !ok && err == nil {
				t.Fatalf("invalid %s/%s accepted", state, action)
			}
		}
	}
}
