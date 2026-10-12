//go:build integration

package integration

import (
	"errors"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
)

func TestRefreshConcurrentGraceAndReuseRevokesFamily(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	account, err := f.service.Register(f.ctx, auth.RegisterInput{Email: "refresh@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	original, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase", RememberMe: true})
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	var wait sync.WaitGroup
	type result struct {
		credentials auth.Credentials
		err         error
	}
	results := make(chan result, 8)
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			credentials, err := f.sessions.Refresh(f.ctx, original.Refresh)
			results <- result{credentials, err}
		}()
	}
	wait.Wait()
	close(results)
	success, conflicts := 0, 0
	var rotated auth.Credentials
	for result := range results {
		if result.err == nil {
			success++
			rotated = result.credentials
			continue
		}
		var problem *platform.APIError
		if !errors.As(result.err, &problem) || problem.Code != "REFRESH_IN_PROGRESS" || result.credentials.Access != "" || result.credentials.Refresh != "" {
			t.Fatal("unexpected concurrent refresh", result.err)
		}
		conflicts++
	}
	if success != 1 || conflicts != 7 || rotated.SessionID != original.SessionID || rotated.Refresh == original.Refresh || !rotated.ExpiresAt.Equal(original.ExpiresAt) {
		t.Fatal("unsafe rotation", success, conflicts)
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM auth_refresh_tokens WHERE session_id=$1`, original.SessionID).Scan(&count); err != nil || count != 2 {
		t.Fatal("duplicate successor tokens", err)
	}
	f.clock.Advance(5 * time.Second)
	if _, err := f.sessions.Refresh(f.ctx, original.Refresh); err == nil {
		t.Fatal("old token accepted at grace boundary")
	}
	if _, err := f.sessions.Authenticate(f.ctx, rotated.Access); err != nil {
		t.Fatal("grace revoked valid successor", err)
	}
	f.clock.Advance(time.Nanosecond)
	if _, err := f.sessions.Refresh(f.ctx, original.Refresh); err == nil {
		t.Fatal("reuse accepted")
	}
	for _, raw := range []string{original.Access, rotated.Access} {
		if _, err := f.sessions.Authenticate(f.ctx, raw); err == nil {
			t.Fatal("reused refresh did not revoke entire session family")
		}
	}
	if _, err := f.sessions.Refresh(f.ctx, rotated.Refresh); err == nil {
		t.Fatal("successor survived family revocation")
	}
	if _, err := f.sessions.Authenticate(f.ctx, other.Access); err != nil {
		t.Fatal("reuse revoked unrelated session", err)
	}
}

func TestHTTPRefreshLogoutAndOwnSessionManagement(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	first, err := f.service.Register(f.ctx, auth.RegisterInput{Email: "owner@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.Register(f.ctx, auth.RegisterInput{Email: "foreign@example.test", Password: "second password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: second.Email, Password: "second password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	client := authClient(t)
	csrf := f.csrf(t, client)
	if got := f.request(t, client, "POST", "/login", csrf, auth.LoginInput{Email: first.Email, Password: "correct password phrase"}); got.status != 200 {
		t.Fatal("login", got.status)
	}
	// Pre-auth token cannot authorize a session mutation after login.
	if got := f.request(t, client, "POST", "/refresh", csrf, struct{}{}); got.status != 403 {
		t.Fatal("preauth authorized refresh", got.status)
	}
	csrf = f.csrf(t, client)
	if got := f.request(t, client, "DELETE", "/sessions/"+foreign.SessionID, csrf, struct{}{}); got.status != 404 {
		t.Fatal("foreign session revocation", got.status)
	}
	secondDevice, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: first.Email, Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.request(t, client, "DELETE", "/sessions/"+secondDevice.SessionID, csrf, struct{}{}); got.status != 200 {
		t.Fatal("own session revocation", got.status)
	}
	if _, err := f.sessions.Authenticate(f.ctx, secondDevice.Access); err == nil {
		t.Fatal("revoked device remains authorized")
	}
	f.clock.Advance(15 * time.Minute)
	if got := f.request(t, client, "GET", "/session", "", nil); got.status != 401 {
		t.Fatal("expired access", got.status)
	}
	if got := f.request(t, client, "POST", "/refresh", csrf, struct{}{}); got.status != 200 {
		t.Fatal("refresh after JWT expiry", got.status)
	}
	if got := f.request(t, client, "GET", "/session", "", nil); got.status != 200 {
		t.Fatal("rotated session", got.status)
	}
	address, _ := url.Parse(f.server.URL)
	snapshot := client.Jar.Cookies(address)
	oldAccess := ""
	for _, cookie := range snapshot {
		if cookie.Name == "ascend_access" {
			oldAccess = cookie.Value
		}
	}
	if got := f.request(t, client, "POST", "/logout", csrf, struct{}{}); got.status != 200 {
		t.Fatal("logout", got.status)
	}
	if _, err := f.sessions.Authenticate(f.ctx, oldAccess); err == nil {
		t.Fatal("JWT works after logout")
	}
	if got := f.request(t, client, "GET", "/session", "", nil); got.status != 401 {
		t.Fatal("logout cookie retained", got.status)
	}
	if _, err := f.sessions.Authenticate(f.ctx, foreign.Access); err == nil {
		t.Fatal("expected naturally expired JWT")
	}
	// Another user's refresh is still alive, even though its JWT expired naturally.
	if _, err := f.sessions.Refresh(f.ctx, foreign.Refresh); err != nil {
		t.Fatal("logout affected foreign family", err)
	}
}

func TestLostRefreshResponseRequiresLoginWithoutMintingOnConflict(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client := authClient(t)
	account, err := f.service.Register(f.ctx, auth.RegisterInput{Email: "lost@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	csrf := f.csrf(t, client)
	if got := f.request(t, client, "POST", "/login", csrf, auth.LoginInput{Email: account.Email, Password: "correct password phrase"}); got.status != 200 {
		t.Fatal(got.status)
	}
	csrf = f.csrf(t, client)
	address, _ := url.Parse(f.server.URL)
	old := client.Jar.Cookies(address)
	f.clock.Advance(15 * time.Minute)
	if got := f.request(t, client, "POST", "/refresh", csrf, struct{}{}); got.status != 200 {
		t.Fatal(got.status)
	}
	// Simulate losing Set-Cookie: restore the request's old opaque token.
	client.Jar.SetCookies(address, old)
	got := f.request(t, client, "POST", "/refresh", csrf, struct{}{})
	if got.status != 409 || len(got.header.Values("Set-Cookie")) != 0 {
		t.Fatal("conflict minted credentials", got.status)
	}
	if got := f.request(t, client, "GET", "/session", "", nil); got.status != 401 {
		t.Fatal("lost response magically recovered", got.status)
	}
	f.clock.Advance(6 * time.Second)
	if got := f.request(t, client, "POST", "/refresh", csrf, struct{}{}); got.status != 401 {
		t.Fatal("stale refresh not rejected", got.status)
	}
}

func TestLogoutAllRevokesEveryOwnedDevice(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client := authClient(t)
	account, err := f.service.Register(f.ctx, auth.RegisterInput{Email: "all@example.test", Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	device, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: account.Email, Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	csrf := f.csrf(t, client)
	if got := f.request(t, client, "POST", "/login", csrf, auth.LoginInput{Email: account.Email, Password: "correct password phrase"}); got.status != 200 {
		t.Fatal(got.status)
	}
	csrf = f.csrf(t, client)
	if got := f.request(t, client, "POST", "/logout-all", csrf, struct{}{}); got.status != http.StatusOK {
		t.Fatal("logout-all", got.status)
	}
	if _, err := f.sessions.Authenticate(f.ctx, device.Access); err == nil {
		t.Fatal("other device survived logout-all")
	}
	if _, err := f.sessions.Refresh(f.ctx, device.Refresh); err == nil {
		t.Fatal("other refresh survived logout-all")
	}
}
