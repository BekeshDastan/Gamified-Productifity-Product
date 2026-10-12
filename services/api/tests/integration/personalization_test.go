//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/profile"
	"reflect"
	"testing"
)

func TestPersonalizationPersistsAcrossDevicesAndValidatesOrder(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	client, account := f.signedIn(t, "appearance@example.test")
	response := f.privateRequest(t, client, "GET", "/me/settings", "", nil)
	var value struct {
		Data profile.Settings `json:"data"`
	}
	if json.Unmarshal(response.body, &value) != nil {
		t.Fatal("settings unavailable")
	}
	if !reflect.DeepEqual(value.Data.Personalization, profile.DefaultPersonalization()) {
		t.Fatal("defaults differ from migration")
	}
	input := value.Data.Preferences
	input.Personalization = profile.Personalization{Theme: "dark", Motion: "reduce", Sound: true, GameMode: "standard", WidgetOrder: []string{"focus", "today", "calendar", "habits", "progress"}}
	if got := f.privateRequest(t, client, "PATCH", "/me/settings", `"1"`, input); got.status != 200 {
		t.Fatal("personalization failed", got.status)
	}
	fresh := authClient(t)
	f.loginAs(t, fresh, account.Email)
	response = f.privateRequest(t, fresh, "GET", "/me/settings", "", nil)
	if json.Unmarshal(response.body, &value) != nil || !reflect.DeepEqual(value.Data.Personalization, input.Personalization) {
		t.Fatal("preferences reset on another device")
	}
	input.Personalization.WidgetOrder = []string{"today", "today", "calendar", "habits", "progress"}
	if got := f.privateRequest(t, client, "PATCH", "/me/settings", `"2"`, input); got.status != 422 {
		t.Fatal("duplicate widgets accepted")
	}
}
