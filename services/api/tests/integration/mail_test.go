//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/mail"
)

func TestMailReachesOnlyLocalReceiver(t *testing.T) {
	t.Parallel()
	address := os.Getenv("TEST_SMTP_ADDR")
	httpBase := os.Getenv("TEST_MAIL_HTTP")
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		t.Fatal("TEST_SMTP_ADDR must explicitly use 127.0.0.1")
	}
	u, err := url.Parse(httpBase)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" {
		t.Fatal("TEST_MAIL_HTTP must explicitly use local Mailpit")
	}
	mailer, err := mail.NewSMTP("test", address, "ascend@example.test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	subject := "ASCEND infrastructure " + hex.EncodeToString(nonce[:])
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mailer.Send(ctx, mail.Message{To: "developer@example.test", Subject: subject, Text: "Локальная проверка. Во внешнюю почту не отправляется."}); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", httpBase+"/api/v1/messages?limit=100", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("Mailpit API unavailable")
	}
	var result struct{ Messages []struct{ Subject string } }
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	for _, message := range result.Messages {
		if message.Subject == subject {
			return
		}
	}
	t.Fatal("test message not found in local receiver")
}
