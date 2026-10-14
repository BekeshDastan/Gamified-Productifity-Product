package notifications

import (
	"bytes"
	"errors"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/mail"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/privateapi"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestMailOutcomesAndDomainSeparatedUnsubscribe(t *testing.T) {
	for _, v := range []struct {
		err   error
		state string
		retry bool
	}{{nil, "provider_accepted", false}, {mail.ErrDeliveryUnknown, "delivery_unknown", false}, {mail.ErrUnavailable, "failed", false}, {mail.ErrTLSRequired, "failed", false}, {&textproto.Error{Code: 550}, "failed", false}, {&textproto.Error{Code: 450}, "retry_wait", true}, {errors.New("connection refused"), "retry_wait", true}} {
		state, _, retry := classifyMail(v.err)
		if state != v.state || retry != v.retry {
			t.Fatal(v, state, retry)
		}
	}
	clock := platform.FixedClock{Time: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)}
	s := &Service{Service: &privateapi.Service{Clock: clock}}
	if e := s.ConfigureDelivery("http://localhost:3000", bytes.Repeat([]byte{12}, 32), nil); e != nil {
		t.Fatal(e)
	}
	owner := "00000000-0000-4000-8000-000000000001"
	token := s.unsubscribeToken(owner, "deadline")
	c, e := s.unsubscribeClaims(token)
	if e != nil || c.Owner != owner || c.Type != "deadline" {
		t.Fatal(c, e)
	}
	if _, e = s.unsubscribeClaims(token + "x"); e == nil {
		t.Fatal("forged token")
	}
	message := s.mailMessage(owner, Notification{Event: Event{Kind: "deadline"}, Title: titles["deadline"], Message: messages["deadline"]}, "person@example.test")
	if !strings.Contains(message.Text, "/notifications/unsubscribe#token=") || strings.Contains(message.Text, "?token=") {
		t.Fatal("token leaked into request URL")
	}
	s.Clock = platform.FixedClock{Time: clock.Time.AddDate(2, 0, 0)}
	if _, e = s.unsubscribeClaims(token); e == nil {
		t.Fatal("expired capability")
	}
}
