//go:build integration

package integration

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/profile"
)

func (f authFixture) privateRequest(t *testing.T, client *http.Client, method, path, version string, input any, keys ...string) authResponse {
	t.Helper()
	csrf := f.csrf(t, client)
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	r, err := http.NewRequest(method, f.server.URL+"/api/v1"+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Origin", "http://localhost:3000")
	r.Header.Set("X-CSRF-Token", csrf)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("If-Match", version)
	key := rand.Text()
	if len(keys) > 0 {
		key = keys[0]
	}
	r.Header.Set("Idempotency-Key", key)
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return authResponse{status: response.StatusCode, header: response.Header, body: data}
}
func (f authFixture) signedIn(t *testing.T, email string) (*http.Client, auth.Account) {
	t.Helper()
	account, err := f.service.Register(f.ctx, auth.RegisterInput{Email: email, Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	client := authClient(t)
	f.loginAs(t, client, email)
	return client, account
}
func (f authFixture) loginAs(t *testing.T, client *http.Client, email string) {
	t.Helper()
	credentials, err := f.sessions.Login(f.ctx, auth.LoginInput{Email: email, Password: "correct password phrase"})
	if err != nil {
		t.Fatal(err)
	}
	recorder := newCookieRecorder(f, credentials)
	address, _ := url.Parse(f.server.URL)
	client.Jar.SetCookies(address, recorder)
}
func newCookieRecorder(f authFixture, credentials auth.Credentials) []*http.Cookie {
	// Reuse the same public cookie policy as HTTP login.
	return []*http.Cookie{{Name: "ascend_access", Value: credentials.Access, Path: "/"}, {Name: "ascend_refresh", Value: credentials.Refresh, Path: "/"}}
}
func TestProfilePersistenceOwnershipAndVersion(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "profile@example.test")
	other, second := f.signedIn(t, "profile-other@example.test")
	input := profile.Input{Name: "Дастан 🌱", Nickname: "dastan", Avatar: "sprout"}
	response := f.privateRequest(t, client, "PATCH", "/me", `"1"`, input)
	if response.status != 200 {
		t.Fatal("profile update failed", response.status)
	}
	var body struct {
		Data profile.Profile `json:"data"`
	}
	if err := json.Unmarshal(response.body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Version != 2 || body.Data.OwnerID != account.ID || body.Data.Name != input.Name {
		t.Fatal("wrong profile saved")
	}
	if result := f.privateRequest(t, client, "PATCH", "/me", `"1"`, input); result.status != 409 {
		t.Fatal("stale overwrite accepted")
	}
	if result := f.privateRequest(t, other, "PATCH", "/me", `"1"`, input); result.status != 422 {
		t.Fatal("duplicate nickname accepted")
	}
	if result := f.privateRequest(t, client, "PATCH", "/me", `"2"`, map[string]any{"name": "foreign", "avatar": "star", "nickname": "", "owner_id": second.ID}); result.status != 422 {
		t.Fatal("owner field accepted")
	}
	if result := f.privateRequest(t, client, "PATCH", fmt.Sprintf("/me/%s", second.ID), `"1"`, input); result.status != 404 {
		t.Fatal("foreign route accepted")
	}
	fresh := authClient(t)
	f.loginAs(t, fresh, account.Email)
	result := f.privateRequest(t, fresh, "GET", "/me", "", nil)
	if result.status != 200 || !bytes.Contains(result.body, []byte(input.Name)) {
		t.Fatal("profile did not survive login")
	}
	result = f.privateRequest(t, other, "GET", "/me", "", nil)
	if bytes.Contains(result.body, []byte(input.Name)) {
		t.Fatal("profile leaked")
	}
}

func TestSettingsPersistAndRemainOwnerScoped(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "settings@example.test")
	other, _ := f.signedIn(t, "settings-other@example.test")
	input := profile.Preferences{Personalization: profile.DefaultPersonalization(), Privacy: profile.DefaultPrivacy(), Timezone: "Asia/Almaty", Language: "en", WeekStart: 7, TimeFormat: "12h", WorkIntervals: []profile.WorkInterval{{Day: 1, Start: "09:00", End: "17:00"}}, RestDays: []int{6, 7}}
	if got := f.privateRequest(t, client, "PATCH", "/me/settings", `"1"`, input); got.status != 200 {
		t.Fatal("settings save failed", got.status)
	}
	if got := f.privateRequest(t, client, "PATCH", "/me/settings", `"1"`, input); got.status != 409 {
		t.Fatal("stale settings accepted")
	}
	fresh := authClient(t)
	f.loginAs(t, fresh, account.Email)
	got := f.privateRequest(t, fresh, "GET", "/me/settings", "", nil)
	if got.status != 200 || !bytes.Contains(got.body, []byte("Asia/Almaty")) {
		t.Fatal("settings lost on new device")
	}
	got = f.privateRequest(t, other, "GET", "/me/settings", "", nil)
	if got.status != 200 || bytes.Contains(got.body, []byte("Asia/Almaty")) {
		t.Fatal("settings leaked to another account")
	}
	input.Timezone = "Local"
	if got := f.privateRequest(t, client, "PATCH", "/me/settings", `"2"`, input); got.status != 422 {
		t.Fatal("invalid timezone accepted")
	}
}
