package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"testing"
	"time"
)

func TestNotificationMalformedJobsArePermanentBeforeStorage(t *testing.T) {
	owner := "00000000-0000-4000-8000-000000000001"
	bad := "bad-owner"
	s := NewService(nil, platform.FixedClock{Time: time.Now()})
	for _, tc := range []struct {
		kind    string
		owner   *string
		payload string
	}{{"notifications.changed", nil, `{}`}, {"notifications.changed", &bad, `{}`}, {"notifications.changed", &owner, `{"unexpected":true}`}, {"notifications.reminders_continue", &owner, `{"after":"bad"}`}, {"notifications.reminders_continue", nil, `{}`}, {"notifications.reminder", &owner, `{"id":"bad","version":1}`}, {"notifications.delivery", &owner, `{"id":"bad","version":1}`}, {"notifications.delivery", nil, `{}`}, {"notifications.reminders_sweep", &owner, `{}`}, {"notifications.reminders_sweep", nil, `{"after":"","bucket":"bad"}`}, {"notifications.reminders_sweep", nil, `{"after":"bad","bucket":"2026-10-12T12:00:00Z"}`}} {
		e := s.Handlers()[tc.kind](context.Background(), nil, jobs.Job{Kind: tc.kind, OwnerID: tc.owner, Payload: json.RawMessage(tc.payload)})
		if !errors.Is(e, jobs.ErrPermanent) {
			t.Fatal(tc, e)
		}
	}
}
