//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/profile"
	"testing"
)

func TestPrivateDefaultsAndNoPublicAccountDisclosure(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "private@example.test")
	anon := authClient(t)
	response := f.privateRequest(t, client, "GET", "/me/settings", "", nil)
	var value struct {
		Data profile.Settings `json:"data"`
	}
	if json.Unmarshal(response.body, &value) != nil || value.Data.Privacy != profile.DefaultPrivacy() {
		t.Fatal("new account is not private")
	}
	for _, path := range []string{"/me", "/me/settings", "/me/onboarding", "/areas", "/tags", "/users", "/profiles/" + account.ID} {
		got := f.privateRequest(t, anon, "GET", path, "", nil)
		if got.status != 401 && got.status != 404 {
			t.Fatal("public private route", path, got.status)
		}
		if bytes.Contains(got.body, []byte(account.Email)) {
			t.Fatal("public email disclosure")
		}
	}
	value.Data.Privacy = profile.Privacy{ProfileVisibility: "friends", ShareActivity: true}
	response = f.privateRequest(t, client, "PATCH", "/me/settings", `"1"`, value.Data.Preferences)
	if response.status != 200 {
		t.Fatal("privacy save failed", response.status)
	}
	response = f.privateRequest(t, client, "GET", "/me", "", nil)
	if bytes.Contains(response.body, []byte("email")) {
		t.Fatal("profile DTO contains email")
	}
	value.Data.Privacy.ProfileVisibility = "public"
	if got := f.privateRequest(t, client, "PATCH", "/me/settings", `"2"`, value.Data.Preferences); got.status != 422 {
		t.Fatal("unsupported public visibility accepted")
	}
}
