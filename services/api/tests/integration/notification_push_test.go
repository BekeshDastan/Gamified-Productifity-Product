//go:build integration

package integration

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/notifications"
	"strings"
	"testing"
)

func TestNotificationPushConsentOwnershipRevokeAndTransfer(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "push-a@example.test")
	b, _ := f.signedIn(t, "push-b@example.test")
	key, e := ecdh.P256().GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	in := notifications.PushInput{Endpoint: "https://fcm.googleapis.com/fcm/send/private-capability", P256dh: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), Auth: base64.RawURLEncoding.EncodeToString(make([]byte, 16))}
	if r := f.privateRequest(t, a, "POST", "/notifications/push", `"0"`, in); r.status != 422 {
		t.Fatal("no consent", r.status)
	}
	in.Confirm = true
	r := f.privateRequest(t, a, "POST", "/notifications/push", `"0"`, in)
	var out struct{ Data notifications.PushDevice }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	d := out.Data
	list := f.privateRequest(t, a, "GET", "/notifications/push", "", nil)
	if strings.Contains(string(list.body), "private-capability") || strings.Contains(string(list.body), in.Auth) {
		t.Fatal("capability leak")
	}
	if r = f.privateRequest(t, b, "POST", "/notifications/push/"+d.ID+"/revoke", `"1"`, map[string]bool{"confirm": true}); r.status != 404 {
		t.Fatal("foreign revoke", r.status)
	}
	changed := in
	changed.Auth = base64.RawURLEncoding.EncodeToString([]byte("different-secret"))
	if r = f.privateRequest(t, b, "POST", "/notifications/push", `"0"`, changed); r.status != 409 {
		t.Fatal("stolen endpoint", r.status)
	}
	if r = f.privateRequest(t, b, "POST", "/notifications/push", `"0"`, in); r.status != 200 {
		t.Fatal("capability transfer", r.status, string(r.body))
	}
	_ = json.Unmarshal(r.body, &out)
	d = out.Data
	if r = f.privateRequest(t, b, "POST", "/notifications/push/"+d.ID+"/revoke", `"0"`, map[string]bool{"confirm": true}); r.status != 409 {
		t.Fatal("stale revoke", r.status)
	}
	if r = f.privateRequest(t, b, "POST", "/notifications/push/"+d.ID+"/revoke", fmt.Sprintf(`"%d"`, d.Version), map[string]bool{"confirm": true}); r.status != 200 {
		t.Fatal(r.status, string(r.body))
	}
}
