//go:build integration

package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
)

func TestLoginSessionsIsolationExpiryAndServerRevocation(t *testing.T) {
	pool, ctx := database(t)
	clock := testkit.NewClock(time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC))
	service := auth.NewService(pool, auth.NewPasswordHasher(), clock)
	sessions, err := auth.NewSessions(ctx, service, auth.SessionConfig{Origin: "https://ascend.example", Environment: "production", SigningKey: bytes.Repeat([]byte{42}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Register(ctx, auth.RegisterInput{Email: "first@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Register(ctx, auth.RegisterInput{Email: "second@example.test", Password: "second password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []auth.LoginInput{{Email: first.Email, Password: "wrong password phrase"}, {Email: "absent@example.test", Password: "correct password phrase"}} {
		if _, err := sessions.Login(ctx, input); err == nil {
			t.Fatal("invalid credentials accepted")
		}
	}
	result, err := sessions.Login(ctx, auth.LoginInput{Email: " FIRST@example.test ", Password: "correct password phrase", RememberMe: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Account.ID != first.ID || result.SessionID == "" || len(result.Refresh) != 43 || result.ExpiresAt.Sub(clock.Now()) != 30*24*time.Hour {
		t.Fatal("invalid session result")
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), result.Access) || strings.Contains(string(data), result.Refresh) {
		t.Fatal("credentials leak in JSON")
	}
	var digest []byte
	if err := pool.QueryRow(ctx, `SELECT token_hash FROM auth_refresh_tokens WHERE session_id=$1`, result.SessionID).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256([]byte(result.Refresh))
	if !bytes.Equal(digest, expected[:]) {
		t.Fatal("refresh was not stored as SHA256")
	}
	identity, err := sessions.Authenticate(ctx, result.Access)
	if err != nil || identity.UserID != first.ID || identity.Role != "user" {
		t.Fatal("session not authenticated", err)
	}
	if problem := platform.RequireOwner(platform.WithIdentity(ctx, identity), second.ID); problem == nil || problem.Status != 404 {
		t.Fatal("account isolation bypassed")
	}
	// Role is read from the database, never from stale claims.
	if _, err := pool.Exec(ctx, `UPDATE users SET role='admin' WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	identity, err = sessions.Authenticate(ctx, result.Access)
	if err != nil || identity.Role != "admin" {
		t.Fatal("stale role", err)
	}
	cookies := httptest.NewRecorder()
	sessions.WriteCookies(cookies, result)
	for _, cookie := range cookies.Result().Cookies() {
		if !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" || cookie.MaxAge <= 0 || !strings.HasPrefix(cookie.Name, "__Host-ascend_") {
			t.Fatal("unsafe production cookie")
		}
	}
	clock.Advance(15 * time.Minute)
	if _, err := sessions.Authenticate(ctx, result.Access); err == nil {
		t.Fatal("expired JWT accepted")
	}
	result, err = sessions.Login(ctx, auth.LoginInput{Email: first.Email, Password: "correct password phrase"})
	if err != nil || result.ExpiresAt.Sub(clock.Now()) != 7*24*time.Hour {
		t.Fatal("default session lifetime", err)
	}
	cookies = httptest.NewRecorder()
	sessions.WriteCookies(cookies, result)
	for _, cookie := range cookies.Result().Cookies() {
		if cookie.MaxAge != 0 || !cookie.Expires.IsZero() {
			t.Fatal("remember-me off must create browser-session cookies")
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE auth_sessions SET revoked_at=$2 WHERE id=$1`, result.SessionID, clock.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Authenticate(ctx, result.Access); err == nil {
		t.Fatal("revoked session JWT accepted")
	}
}

func TestFailedLoginAuditRollsBackSession(t *testing.T) {
	pool, ctx := database(t)
	service := auth.NewService(pool, auth.NewPasswordHasher(), platform.RealClock{})
	sessions, err := auth.NewSessions(ctx, service, auth.SessionConfig{Origin: "http://localhost:3000", Environment: "test", InsecureLocalCookies: true, SigningKey: bytes.Repeat([]byte{21}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.Register(ctx, auth.RegisterInput{Email: "rollback-login@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE audit_events ADD CONSTRAINT test_reject_login CHECK(action <> 'auth.logged_in')`); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Login(ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"}); err == nil {
		t.Fatal("audit failure accepted")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM auth_sessions`).Scan(&count); err != nil || count != 0 {
		t.Fatal("session survived failed transaction", err)
	}
}
