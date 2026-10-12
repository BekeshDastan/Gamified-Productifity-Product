//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
)

func TestRecoveryNeutralResponseEncryptedQueueAndSessionInvalidation(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client := authClient(t)
	mailer := &testkit.FakeMailer{}
	account, err := f.service.Register(f.ctx, auth.RegisterInput{Email: "recover@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	csrf := f.csrf(t, client)
	var knownData, unknownData map[string]any
	for _, email := range []string{account.Email, "unknown@example.test"} {
		response := f.request(t, client, "POST", "/forgot-password", csrf, auth.ForgotInput{Email: email})
		if response.status != 200 {
			t.Fatal("forgot", response.status)
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(response.body, &envelope); err != nil {
			t.Fatal(err)
		}
		if email == account.Email {
			knownData = envelope.Data
		} else {
			unknownData = envelope.Data
		}
	}
	known, _ := json.Marshal(knownData)
	unknown, _ := json.Marshal(unknownData)
	if !bytes.Equal(known, unknown) {
		t.Fatal("forgot discloses account existence")
	}
	var requests int
	var encrypted bool
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*),bool_and(position('email' in payload::text)=0 AND position('@example.test' in payload::text)=0) FROM outbox_events WHERE kind='auth.reset_request'`).Scan(&requests, &encrypted); err != nil || requests != 2 || !encrypted {
		t.Fatal("forgot path was not uniform/private", err)
	}
	deliverAuthMail(t, f, mailer)
	if len(mailer.Messages()) != 1 {
		t.Fatal("unknown email was sent a reset")
	}
	token := tokenFromMail(t, mailer)
	if err := f.sessions.VerifyEmail(f.ctx, token); err == nil {
		t.Fatal("reset token accepted for verification")
	}
	const newPassword = "brand new password phrase"
	if got := f.request(t, client, "POST", "/reset-password", csrf, auth.ResetInput{Token: token, Password: newPassword}); got.status != 200 {
		t.Fatal("reset", got.status)
	}
	for _, credentials := range []auth.Credentials{first, second} {
		if _, err := f.sessions.Authenticate(f.ctx, credentials.Access); err == nil {
			t.Fatal("old access survived reset")
		}
		if _, err := f.sessions.Refresh(f.ctx, credentials.Refresh); err == nil {
			t.Fatal("old refresh survived reset")
		}
	}
	if _, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"}); err == nil {
		t.Fatal("old password accepted")
	}
	if _, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: newPassword}); err != nil {
		t.Fatal("new password rejected", err)
	}
	if err := f.sessions.ResetPassword(f.ctx, auth.ResetInput{Token: token, Password: "another valid password phrase"}); err == nil {
		t.Fatal("reset link accepted twice")
	}
	if got := f.request(t, client, "POST", "/forgot-password", f.csrf(t, client), auth.ForgotInput{Email: account.Email}); got.status != 200 {
		t.Fatal(got.status)
	}
	if got := f.request(t, client, "POST", "/forgot-password", f.csrf(t, client), auth.ForgotInput{Email: account.Email}); got.status != 429 {
		t.Fatal("reset request unlimited", got.status)
	}
}

func TestPasswordResetExpiresAtThirtyMinutes(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	mailer := &testkit.FakeMailer{}
	_, err := f.service.Register(f.ctx, auth.RegisterInput{Email: "reset-expired@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.sessions.ForgotPassword(f.ctx, auth.ForgotInput{Email: "reset-expired@example.test"}); err != nil {
		t.Fatal(err)
	}
	deliverAuthMail(t, f, mailer)
	token := tokenFromMail(t, mailer)
	f.clock.Advance(30 * time.Minute)
	if err := f.sessions.ResetPassword(f.ctx, auth.ResetInput{Token: token, Password: "brand new password phrase"}); err == nil {
		t.Fatal("expired reset accepted")
	}
}

func TestPasswordResetAuditFailureRollsBackPasswordAndRevocation(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	mailer := &testkit.FakeMailer{}
	account, err := f.service.Register(f.ctx, auth.RegisterInput{Email: "reset-rollback@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.sessions.ForgotPassword(f.ctx, auth.ForgotInput{Email: account.Email}); err != nil {
		t.Fatal(err)
	}
	deliverAuthMail(t, f, mailer)
	token := tokenFromMail(t, mailer)
	if _, err := f.pool.Exec(f.ctx, `ALTER TABLE audit_events ADD CONSTRAINT test_reset_audit CHECK(action <> 'auth.password_reset')`); err != nil {
		t.Fatal(err)
	}
	if err := f.sessions.ResetPassword(f.ctx, auth.ResetInput{Token: token, Password: "brand new password phrase"}); err == nil {
		t.Fatal("reset ignored audit failure")
	}
	if _, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"}); err != nil {
		t.Fatal("failed reset changed password", err)
	}
	if _, err := f.sessions.Authenticate(f.ctx, credentials.Access); err != nil {
		t.Fatal("failed reset revoked session", err)
	}
}
