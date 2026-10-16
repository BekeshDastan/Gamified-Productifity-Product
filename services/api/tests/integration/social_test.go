//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/social"
	"net/http"
	"testing"
)

func socialRead[T any](t *testing.T, r authResponse) T {
	t.Helper()
	var v struct{ Data T }
	if r.status != 200 || json.Unmarshal(r.body, &v) != nil {
		t.Fatalf("%d %s", r.status, r.body)
	}
	return v.Data
}
func socialVerify(t *testing.T, f authFixture, id string) {
	t.Helper()
	if _, e := f.pool.Exec(f.ctx, "UPDATE users SET email_verified_at=$2 WHERE id=$1", id, f.clock.Now()); e != nil {
		t.Fatal(e)
	}
}
func socialLink(t *testing.T, f authFixture, a, b *http.Client) string {
	t.Helper()
	v := socialRead[social.Friends](t, f.privateRequest(t, b, "GET", "/social/friends", "", nil))
	r := socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/friends", "", social.FriendAction{Action: "request", Code: v.Code}))
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/friends", "", social.FriendAction{Action: "accept", ID: r.ID, Version: 1}))
	return r.ID
}
func TestSocialFriendsRequireConsentAndVerifiedSender(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "social-a@example.test")
	b, other := f.signedIn(t, "social-b@example.test")
	c, _ := f.signedIn(t, "social-c@example.test")
	v := socialRead[social.Friends](t, f.privateRequest(t, b, "GET", "/social/friends", "", nil))
	in := social.FriendAction{Action: "request", Code: v.Code}
	if r := f.privateRequest(t, a, "POST", "/social/friends", "", in); r.status != 403 {
		t.Fatal(r.status, string(r.body))
	}
	socialVerify(t, f, who.ID)
	if r := f.privateRequest(t, a, "POST", "/social/friends", "", social.FriendAction{Action: "request", Code: other.ID}); r.status != 404 {
		t.Fatal("ID is not an invitation", r.status)
	}
	receipt := socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/friends", "", in))
	accept := social.FriendAction{Action: "accept", ID: receipt.ID, Version: 1}
	for _, client := range []*http.Client{a, c} {
		if r := f.privateRequest(t, client, "POST", "/social/friends", "", accept); r.status == 200 {
			t.Fatal("unauthorized acceptance")
		}
	}
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/friends", "", accept))
	if r := f.privateRequest(t, b, "POST", "/social/friends", "", accept); r.status != 409 {
		t.Fatal("stale acceptance", r.status)
	}
	list := socialRead[social.Friends](t, f.privateRequest(t, a, "GET", "/social/friends", "", nil))
	if len(list.Items) != 1 || list.Items[0].State != "accepted" {
		t.Fatal(list)
	}
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/friends", "", social.FriendAction{Action: "remove", ID: receipt.ID, Version: 2}))
	list = socialRead[social.Friends](t, f.privateRequest(t, a, "GET", "/social/friends", "", nil))
	if len(list.Items) != 0 {
		t.Fatal("removed friend still visible")
	}
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/friends", "", social.FriendAction{Action: "rotate"}))
	if r := f.privateRequest(t, a, "POST", "/social/friends", "", in); r.status != 404 {
		t.Fatal("rotated invite usable")
	}
}
