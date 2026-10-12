//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
)

func TestCurrentPasswordRecheckAndRevocation(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "change@example.test")
	other, _ := f.signedIn(t, "change-other@example.test")
	credentials, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	mailer := &testkit.FakeMailer{}
	if err := f.sessions.ForgotPassword(f.ctx, auth.ForgotInput{Email: account.Email}); err != nil {
		t.Fatal(err)
	}
	deliverAuthMail(t, f, mailer)
	token := tokenFromMail(t, mailer)
	input := auth.ChangePasswordInput{CurrentPassword: "wrong current password", Password: "brand new password phrase"}
	if got := f.privateRequest(t, client, "PATCH", "/me/password", "", input); got.status != 422 {
		t.Fatal("wrong current password accepted", got.status)
	}
	if _, err := f.sessions.Authenticate(f.ctx, credentials.Access); err != nil {
		t.Fatal("wrong password revoked sessions")
	}
	input.CurrentPassword = "correct password phrase"
	if got := f.privateRequest(t, client, "PATCH", "/me/password", "", input); got.status != 200 {
		t.Fatal("password change failed", got.status)
	}
	if _, err := f.sessions.Authenticate(f.ctx, credentials.Access); err == nil {
		t.Fatal("other device access survived")
	}
	if _, err := f.sessions.Refresh(f.ctx, credentials.Refresh); err == nil {
		t.Fatal("other device refresh survived")
	}
	if got := f.request(t, client, "GET", "/session", "", nil); got.status != 401 {
		t.Fatal("current device survived")
	}
	if got := f.request(t, other, "GET", "/session", "", nil); got.status != 200 {
		t.Fatal("other account was revoked")
	}
	if _, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"}); err == nil {
		t.Fatal("old password accepted")
	}
	if _, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: input.Password}); err != nil {
		t.Fatal("new password rejected", err)
	}
	if err := f.sessions.ResetPassword(f.ctx, auth.ResetInput{Token: token, Password: "another password phrase"}); err == nil {
		t.Fatal("old reset link survived")
	}
}
func TestPasswordChangeRateLimitAndAuditRollback(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "change-rollback@example.test")
	wrong := auth.ChangePasswordInput{CurrentPassword: "wrong password", Password: "brand new password phrase"}
	for i := 0; i < 5; i++ {
		if got := f.privateRequest(t, client, "PATCH", "/me/password", "", wrong); got.status != 422 {
			t.Fatal("wrong attempt response", got.status)
		}
	}
	if got := f.privateRequest(t, client, "PATCH", "/me/password", "", wrong); got.status != 429 || got.header.Get("Retry-After") == "" {
		t.Fatal("unlimited password guesses")
	}
	f.clock.Advance(15 * time.Minute)
	// New access after its expiry; separate service call proves transactional rollback.
	credentials, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := f.sessions.Authenticate(f.ctx, credentials.Access)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `ALTER TABLE audit_events ADD CONSTRAINT test_change_audit CHECK(action <> 'auth.password_changed')`); err != nil {
		t.Fatal(err)
	}
	if err := f.sessions.ChangePassword(platform.WithIdentity(f.ctx, identity), auth.ChangePasswordInput{CurrentPassword: "correct password phrase", Password: wrong.Password}); err == nil {
		t.Fatal("audit failure ignored")
	}
	if _, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"}); err != nil {
		t.Fatal("failed transaction changed password")
	}
	if _, err := f.sessions.Authenticate(f.ctx, credentials.Access); err != nil {
		t.Fatal("failed transaction revoked session")
	}
}
