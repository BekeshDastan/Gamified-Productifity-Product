package notifications

import "testing"

func TestNotificationEventsRejectForeignDestinationsAndUnknownTypes(t *testing.T) {
	base := Event{Kind: "deadline", SourceKind: "task", SourceID: "task", SourceRevision: 1, EventKey: "task/1", URL: "/app/quests/task"}
	if validateEvent(base) != nil {
		t.Fatal("valid event")
	}
	for _, change := range []func(*Event){func(e *Event) { e.Kind = "code" }, func(e *Event) { e.URL = "https://example.test" }, func(e *Event) { e.URL = "/app/../private" }, func(e *Event) { e.URL = "/app/\\evil" }, func(e *Event) { e.SourceRevision = 0 }, func(e *Event) { e.SourceID = "" }, func(e *Event) { e.EventKey = "" }} {
		v := base
		change(&v)
		if validateEvent(v) == nil {
			t.Fatal(v)
		}
	}
}
