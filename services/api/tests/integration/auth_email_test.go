//go:build integration

package integration

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"github.com/jackc/pgx/v5"
)

func deliverAuthMail(t *testing.T, f authFixture, mailer *testkit.FakeMailer) {
	t.Helper()
	store := jobs.Store{Pool: f.pool, Clock: f.clock}
	if _, err := store.Dispatch(f.ctx, 100); err != nil {
		t.Fatal(err)
	}
	for {
		job, err := store.Claim(f.ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Process(f.ctx, job, f.sessions.MailHandlers(mailer)); err != nil {
			t.Fatal(err)
		}
	}
}

func tokenFromMail(t *testing.T, mailer *testkit.FakeMailer) string {
	t.Helper()
	messages := mailer.Messages()
	if len(messages) == 0 {
		t.Fatal("mail was not delivered")
	}
	text := messages[len(messages)-1].Text
	pieces := strings.Split(text, "#token=")
	if len(pieces) != 2 {
		t.Fatal("mail link does not use URL fragment")
	}
	token := strings.Split(pieces[1], "\n")[0]
	if len(token) != 43 {
		t.Fatal("invalid opaque mail token")
	}
	return token
}

func TestVerificationOutboxOneTimeConcurrencyAndExpiry(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	mailer := &testkit.FakeMailer{}
	account, err := f.sessions.Register(f.ctx, auth.RegisterInput{Email: "verify@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	deliverAuthMail(t, f, mailer)
	token := tokenFromMail(t, mailer)
	digest := sha256.Sum256([]byte(token))
	var matches bool
	if err := f.pool.QueryRow(f.ctx, `SELECT token_hash=$1 AND expires_at-created_at=interval '24 hours' FROM auth_one_time_tokens WHERE user_id=$2`, digest[:], account.ID).Scan(&matches); err != nil || !matches {
		t.Fatal("one-time token was not hashed/expiring", err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM outbox_events WHERE position($1 in payload::text)>0) OR EXISTS(SELECT 1 FROM jobs WHERE position($1 in payload::text)>0)`, token).Scan(&matches); err != nil || matches {
		t.Fatal("raw token leaked to outbox/jobs", err)
	}
	var wait sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() { defer wait.Done(); results <- f.sessions.VerifyEmail(f.ctx, token) }()
	}
	wait.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
			continue
		}
		var problem *platform.APIError
		if !errors.As(err, &problem) || problem.Code != "TOKEN_INVALID" {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("verification consumed more than once", success)
	}
	var verified bool
	if err := f.pool.QueryRow(f.ctx, `SELECT email_verified_at IS NOT NULL FROM users WHERE id=$1`, account.ID).Scan(&verified); err != nil || !verified {
		t.Fatal("account not verified", err)
	}
	_, err = f.sessions.Register(f.ctx, auth.RegisterInput{Email: "expired@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	deliverAuthMail(t, f, mailer)
	expired := tokenFromMail(t, mailer)
	f.clock.Advance(24 * time.Hour)
	if err := f.sessions.VerifyEmail(f.ctx, expired); err == nil {
		t.Fatal("expired verification link accepted")
	}
}

func TestResendInvalidatesPreviousLinkAndLimitsRequests(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	mailer := &testkit.FakeMailer{}
	client := authClient(t)
	account, err := f.sessions.Register(f.ctx, auth.RegisterInput{Email: "resend@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	deliverAuthMail(t, f, mailer)
	old := tokenFromMail(t, mailer)
	csrf := f.csrf(t, client)
	if got := f.request(t, client, "POST", "/login", csrf, auth.LoginInput{Email: account.Email, Password: "correct password phrase"}); got.status != 200 {
		t.Fatal(got.status)
	}
	csrf = f.csrf(t, client)
	for i := 0; i < 3; i++ {
		if got := f.request(t, client, "POST", "/resend-verification", csrf, struct{}{}); got.status != 200 {
			t.Fatal("resend", got.status)
		}
	}
	if got := f.request(t, client, "POST", "/resend-verification", csrf, struct{}{}); got.status != 429 {
		t.Fatal("resend unlimited", got.status)
	}
	deliverAuthMail(t, f, mailer)
	fresh := tokenFromMail(t, mailer)
	if old == fresh {
		t.Fatal("resend reused link")
	}
	if err := f.sessions.VerifyEmail(f.ctx, old); err == nil {
		t.Fatal("old link survived resend")
	}
	if err := f.sessions.VerifyEmail(f.ctx, fresh); err != nil {
		t.Fatal("fresh link rejected", err)
	}
}

func TestRegistrationMailQueueFailureRollsBackAccount(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	if _, err := f.pool.Exec(f.ctx, `ALTER TABLE outbox_events ADD CONSTRAINT test_reject_mail CHECK(kind <> 'auth.mail.verify_email')`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.sessions.Register(f.ctx, auth.RegisterInput{Email: "no-mail@example.test", Password: "correct password phrase"}); err == nil {
		t.Fatal("registration ignored mail queue failure")
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil || count != 0 {
		t.Fatal("undeliverable registration persisted", err)
	}
}

func TestUnverifiedAccountCannotPublish(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	account, err := f.sessions.Register(f.ctx, auth.RegisterInput{Email: "publication@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := f.sessions.Authenticate(f.ctx, credentials.Access)
	if err != nil {
		t.Fatal(err)
	}
	req := httptestRequestWithIdentity(f, identity)
	recorder := httptest.NewRecorder()
	published := false
	handler := f.sessions.RequireVerified(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { published = true }))
	handler.ServeHTTP(recorder, req)
	if recorder.Code != 403 || published {
		t.Fatal("unverified publication allowed")
	}
	mailer := &testkit.FakeMailer{}
	deliverAuthMail(t, f, mailer)
	if err := f.sessions.VerifyEmail(f.ctx, tokenFromMail(t, mailer)); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if !published {
		t.Fatal("verified account blocked")
	}
}

func httptestRequestWithIdentity(f authFixture, identity platform.Identity) *http.Request {
	return httptest.NewRequest("POST", "http://localhost:3000/test-publication", nil).WithContext(platform.WithIdentity(f.ctx, identity))
}
