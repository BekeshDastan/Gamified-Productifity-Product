//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/social"
	"net/http"
	"testing"
	"time"
)

func TestSocialCoopConsentCorrectionsAndLeaving(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "coop-a@example.test")
	b, other := f.signedIn(t, "coop-b@example.test")
	socialVerify(t, f, who.ID)
	socialVerify(t, f, other.ID)
	socialLink(t, f, a, b)
	in := social.RoomAction{Action: "create", Name: "Читаем вместе", Mode: "coop", Metric: "manual", Unit: "pages", Target: 20, DailyCap: 10, Days: 3, Consent: true}
	id := socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", in)).ID
	socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", social.RoomAction{Action: "invite", ID: id, Recipient: other.ID}))
	list := socialRead[[]social.Room](t, f.privateRequest(t, b, "GET", "/social/rooms", "", nil))
	if len(list) != 1 || len(list[0].Members) != 0 {
		t.Fatal("unaccepted invitation exposes progress", list)
	}
	join := social.RoomAction{Action: "join", ID: id, RulesHash: list[0].RulesHash, Version: 1}
	if r := f.privateRequest(t, b, "POST", "/social/rooms", "", join); r.status != 409 {
		t.Fatal("missing consent", r.status)
	}
	join.Consent = true
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/rooms", "", join))
	check := social.RoomAction{Action: "checkin", ID: id, Amount: 8}
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/rooms", "", check))
	f.clock.Advance(time.Second)
	list = socialRead[[]social.Room](t, f.privateRequest(t, a, "GET", "/social/rooms", "", nil))
	if list[0].Total != 8 {
		t.Fatal(list)
	}
	if r := f.privateRequest(t, b, "POST", "/social/rooms", "", check); r.status != 409 {
		t.Fatal("duplicate checkin", r.status)
	}
	check.Version = 1
	check.Amount = 3
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/rooms", "", check))
	list = socialRead[[]social.Room](t, f.privateRequest(t, a, "GET", "/social/rooms", "", nil))
	if list[0].Total != 3 {
		t.Fatal("correction", list)
	}
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/rooms", "", social.RoomAction{Action: "leave", ID: id, Version: 2}))
	list = socialRead[[]social.Room](t, f.privateRequest(t, a, "GET", "/social/rooms", "", nil))
	if list[0].Total != 0 {
		t.Fatal("left contribution leaked", list)
	}
	if _, e := f.pool.Exec(f.ctx, "UPDATE social_rooms SET target=999 WHERE id=$1", id); e == nil {
		t.Fatal("mutable rules")
	}
}

func TestSocialPvPTiesFinalizationAndLeaderboardConsent(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "race-a@example.test")
	b, other := f.signedIn(t, "race-b@example.test")
	socialVerify(t, f, who.ID)
	socialVerify(t, f, other.ID)
	socialLink(t, f, a, b)
	in := social.RoomAction{Action: "create", Name: "Чтение", Mode: "pvp", Metric: "manual", Unit: "pages", Target: 5, DailyCap: 10, Days: 1, Consent: true, Leaderboard: true}
	id := socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", in)).ID
	socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", social.RoomAction{Action: "invite", ID: id, Recipient: other.ID}))
	list := socialRead[[]social.Room](t, f.privateRequest(t, b, "GET", "/social/rooms", "", nil))
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/rooms", "", social.RoomAction{Action: "join", ID: id, Version: 1, RulesHash: list[0].RulesHash, Consent: true}))
	if r := f.privateRequest(t, a, "POST", "/social/rooms", "", social.RoomAction{Action: "checkin", ID: id, Amount: 5}); r.status != 422 {
		t.Fatal("early start", r.status)
	}
	f.clock.Advance(12*time.Hour + time.Second)
	f.loginAs(t, a, "race-a@example.test")
	f.loginAs(t, b, "race-b@example.test")
	for _, client := range []*http.Client{a, b} {
		socialRead[social.Receipt](t, f.privateRequest(t, client, "POST", "/social/rooms", "", social.RoomAction{Action: "checkin", ID: id, Amount: 5}))
	}
	f.clock.Advance(time.Second)
	list = socialRead[[]social.Room](t, f.privateRequest(t, a, "GET", "/social/rooms", "", nil))
	if len(list[0].Members) != 1 {
		t.Fatal("leaderboard requires separate consent", list)
	}
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/rooms", "", social.RoomAction{Action: "sharing", ID: id, Version: 2, Consent: true, Leaderboard: true}))
	list = socialRead[[]social.Room](t, f.privateRequest(t, a, "GET", "/social/rooms", "", nil))
	if len(list[0].Members) != 2 || list[0].Members[0].Rank != 1 || list[0].Members[1].Rank != 1 {
		t.Fatal("ties", list)
	}
	f.clock.Advance(24 * time.Hour)
	f.loginAs(t, a, "race-a@example.test")
	f.loginAs(t, b, "race-b@example.test")
	for i := 0; i < 3; i++ {
		socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", social.RoomAction{Action: "finalize", ID: id}))
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM social_results WHERE room_id=$1 AND earned", id).Scan(&n); e != nil || n != 2 {
		t.Fatal("reward duplicated or tie lost", n, e)
	}
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/rooms", "", social.RoomAction{Action: "sharing", ID: id, Version: 3, Consent: false}))
	socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", social.RoomAction{Action: "finalize", ID: id}))
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM social_results WHERE room_id=$1 AND earned", id).Scan(&n); e != nil || n != 0 {
		t.Fatal("revoked standings remain", n, e)
	}
}

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

func TestSocialSharingIsExplicitAndRevokedOnRemoval(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "grant-a@example.test")
	b, other := f.signedIn(t, "grant-b@example.test")
	c, _ := f.signedIn(t, "grant-c@example.test")
	socialVerify(t, f, who.ID)
	id := socialLink(t, f, a, b)
	in := social.GrantAction{Recipient: other.ID, Metric: "regularity", Allow: true}
	socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/grants", "", in))
	v := socialRead[[]social.Grant](t, f.privateRequest(t, b, "GET", "/social/grants", "", nil))
	if len(v) != 1 || v[0].Owner != who.ID {
		t.Fatal(v)
	}
	v = socialRead[[]social.Grant](t, f.privateRequest(t, c, "GET", "/social/grants", "", nil))
	if len(v) != 0 {
		t.Fatal("foreign summary", v)
	}
	in.Allow = false
	socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/grants", "", in))
	v = socialRead[[]social.Grant](t, f.privateRequest(t, b, "GET", "/social/grants", "", nil))
	if len(v) != 0 {
		t.Fatal("revoked summary", v)
	}
	in.Allow = true
	in.Metric = "journal"
	if r := f.privateRequest(t, a, "POST", "/social/grants", "", in); r.status != 422 {
		t.Fatal("non allowlisted metric", r.status)
	}
	in.Metric = "evening"
	socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/grants", "", in))
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/friends", "", social.FriendAction{Action: "remove", ID: id, Version: 2}))
	v = socialRead[[]social.Grant](t, f.privateRequest(t, b, "GET", "/social/grants", "", nil))
	if len(v) != 0 {
		t.Fatal("friend removal must revoke", v)
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM social_grants").Scan(&n); e != nil || n != 0 {
		t.Fatal("grant resurrection", n, e)
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
