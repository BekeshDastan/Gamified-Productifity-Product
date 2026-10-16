//go:build integration

package integration

import (
	"bufio"
	"context"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/focus"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/journal"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/social"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSocialRecordedFocusAndReviewsRespectSources(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "recorded-social@example.test")
	socialVerify(t, f, who.ID)
	for _, metric := range []string{"focus", "reviews"} {
		unit, cap := "minutes", int64(2)
		if metric == "reviews" {
			unit, cap = "reviews", 1
		}
		socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", social.RoomAction{Action: "create", Name: metric, Mode: "coop", Metric: metric, Unit: unit, Target: cap, DailyCap: cap, Days: 2, Consent: true}))
	}
	check := func(metric string, want int64) {
		t.Helper()
		rooms := socialRead[[]social.Room](t, f.privateRequest(t, a, "GET", "/social/rooms", "", nil))
		for _, room := range rooms {
			if room.Metric == metric && room.Total != want {
				t.Fatalf("%s got %d want %d", metric, room.Total, want)
			}
		}
	}
	f.clock.Advance(time.Second)
	s := readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", focusInput()))
	f.clock.Advance(90 * time.Second)
	s = focusAct(t, f, a, s, "pause")
	f.clock.Advance(5 * time.Minute)
	s = focusAct(t, f, a, s, "resume")
	f.clock.Advance(90 * time.Second)
	s = focusAct(t, f, a, s, "finish")
	check("focus", 0)
	readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), focus.ConfirmInput{Result: "progress"}))
	f.clock.Advance(time.Second)
	check("focus", 2) // Three worked minutes, capped at two; pause excluded.
	s = readFocus(t, f.privateRequest(t, a, "POST", "/focus/start", "", focusInput()))
	f.clock.Advance(time.Minute)
	s = focusAct(t, f, a, s, "finish")
	readFocus(t, f.privateRequest(t, a, "POST", "/focus/sessions/"+s.ID+"/confirm", focusVersion(s), focus.ConfirmInput{Result: "rest"}))
	check("focus", 2)
	path := "/reviews/day/" + f.clock.Now().UTC().Format("2006-01-02")
	r := readReview(t, f.privateRequest(t, a, "PUT", path, `"0"`, journal.ReviewInput{Obstacle: "none", Conclusion: "PRIVATE SOCIAL REVIEW"}))
	f.clock.Advance(time.Second)
	check("reviews", 1)
	readReview(t, f.privateRequest(t, a, "PUT", path, fmt.Sprintf(`"%d"`, r.Run.Version), journal.ReviewInput{Obstacle: "none", Conclusion: "PRIVATE CORRECTION"}))
	check("reviews", 1)
	response := f.privateRequest(t, a, "GET", "/social/dashboard", "", nil)
	if strings.Contains(string(response.body), "PRIVATE") {
		t.Fatal("review content leaked")
	}
}

func TestSocialCoopAchievementReconcilesWithoutOpeningFriends(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "coop-achievement-a@example.test")
	b, other := f.signedIn(t, "coop-achievement-b@example.test")
	c, third := f.signedIn(t, "coop-achievement-c@example.test")
	socialVerify(t, f, who.ID)
	socialVerify(t, f, other.ID)
	socialVerify(t, f, third.ID)
	socialLink(t, f, a, b)
	socialLink(t, f, a, c)
	id := socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", social.RoomAction{Action: "create", Name: "Два шага", Mode: "coop", Metric: "tasks", Unit: "tasks", Days: 1, DailyCap: 2, Target: 2, Consent: true})).ID
	socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", social.RoomAction{Action: "invite", ID: id, Recipient: other.ID}))
	rooms := socialRead[[]social.Room](t, f.privateRequest(t, b, "GET", "/social/rooms", "", nil))
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/rooms", "", social.RoomAction{Action: "join", ID: id, Version: 1, RulesHash: rooms[0].RulesHash, Consent: true}))
	socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", social.RoomAction{Action: "invite", ID: id, Recipient: third.ID}))
	socialRead[social.Receipt](t, f.privateRequest(t, c, "POST", "/social/rooms", "", social.RoomAction{Action: "join", ID: id, Version: 1, RulesHash: rooms[0].RulesHash, Consent: true}))
	f.clock.Advance(time.Second)
	qa := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Личная задача А")))
	qa = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+qa.ID+"/complete", version(qa), map[string]any{}))
	qb := readQuest(t, f.privateRequest(t, b, "POST", "/quests", "", questInput("Личная задача Б")))
	qb = readQuest(t, f.privateRequest(t, b, "POST", "/quests/"+qb.ID+"/complete", version(qb), map[string]any{}))
	qc := readQuest(t, f.privateRequest(t, c, "POST", "/quests", "", questInput("Личная задача В")))
	readQuest(t, f.privateRequest(t, c, "POST", "/quests/"+qc.ID+"/complete", version(qc), map[string]any{}))
	f.clock.Advance(24 * time.Hour)
	f.loginAs(t, a, "coop-achievement-a@example.test")
	f.loginAs(t, b, "coop-achievement-b@example.test")
	f.loginAs(t, c, "coop-achievement-c@example.test")
	socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", social.RoomAction{Action: "finalize", ID: id}))
	check := func(client *http.Client, want bool) {
		t.Helper()
		values := socialRead[[]struct {
			ID     string `json:"id"`
			Active bool   `json:"active"`
		}](t, f.privateRequest(t, client, "GET", "/achievements", "", nil))
		for _, v := range values {
			if v.ID == "coop-1" {
				if v.Active != want {
					t.Fatal("stale co-op achievement", v)
				}
				return
			}
		}
		t.Fatal("missing co-op definition")
	}
	check(a, true)
	check(b, true)
	socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", social.RoomAction{Action: "leave", ID: id, Version: 1}))
	check(a, false)
	check(b, true)
	check(c, true)
	readQuest(t, f.privateRequest(t, b, "POST", "/quests/"+qb.ID+"/reopen", version(qb), map[string]any{"reason": "Correction"}))
	check(b, false)
	check(c, false)
}

func TestSocialUnwantedInvitationReportBlockAndAdminResolution(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "unwanted-a@example.test")
	b, other := f.signedIn(t, "unwanted-b@example.test")
	socialVerify(t, f, who.ID)
	code := socialRead[social.Friends](t, f.privateRequest(t, b, "GET", "/social/friends", "", nil)).Code
	socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/friends", "", social.FriendAction{Action: "request", Code: code}))
	for i := 0; i < 5; i++ {
		socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/moderation", "", social.ModerationAction{Action: "report", Target: who.ID, Reason: "spam"}))
	}
	if r := f.privateRequest(t, b, "POST", "/social/moderation", "", social.ModerationAction{Action: "report", Target: who.ID, Reason: "spam"}); r.status != 429 {
		t.Fatal(r.status, string(r.body))
	}
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/moderation", "", social.ModerationAction{Action: "block", Target: who.ID}))
	if list := socialRead[social.Friends](t, f.privateRequest(t, b, "GET", "/social/friends", "", nil)); len(list.Items) != 0 {
		t.Fatal(list)
	}
	if _, e := f.pool.Exec(f.ctx, "UPDATE users SET role='admin' WHERE id=$1", other.ID); e != nil {
		t.Fatal(e)
	}
	m := socialRead[social.Moderation](t, f.privateRequest(t, b, "GET", "/social/moderation", "", nil))
	resolvedID := m.Reports[0].ID
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/moderation", "", social.ModerationAction{Action: "resolve", ID: resolvedID}))
	m = socialRead[social.Moderation](t, f.privateRequest(t, b, "GET", "/social/moderation", "", nil))
	for _, report := range m.Reports {
		if report.ID == resolvedID && !report.Resolved {
			t.Fatal("report remains open")
		}
	}
}

func TestSocialInvalidationStreamCarriesNoSharedData(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "stream@example.test")
	socialRead[social.Friends](t, f.privateRequest(t, a, "GET", "/social/friends", "", nil))
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "GET", f.server.URL+"/api/v1/social/changes", nil)
	response, e := a.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Cache-Control") != "no-store, no-transform" {
		t.Fatal(response.StatusCode, response.Header)
	}
	reader := bufio.NewReader(response.Body)
	next := func() string {
		t.Helper()
		for {
			line, e := reader.ReadString('\n')
			if e != nil {
				t.Fatal(e)
			}
			if strings.HasPrefix(line, "data: ") {
				token := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
				if len(token) != 36 || strings.Contains(token, "@") {
					t.Fatal("invalid stream payload")
				}
				return token
			}
		}
	}
	first := next()
	socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/friends", "", social.FriendAction{Action: "rotate"}))
	if next() == first {
		t.Fatal("revocation marker did not change")
	}
}

func TestSocialRejectedInvitesCooldownAndRoomLimit(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "reject-social-a@example.test")
	b, other := f.signedIn(t, "reject-social-b@example.test")
	socialVerify(t, f, who.ID)
	socialVerify(t, f, other.ID)
	code := socialRead[social.Friends](t, f.privateRequest(t, b, "GET", "/social/friends", "", nil)).Code
	id := socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/friends", "", social.FriendAction{Action: "request", Code: code})).ID
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/friends", "", social.FriendAction{Action: "reject", ID: id, Version: 1}))
	if r := f.privateRequest(t, a, "POST", "/social/friends", "", social.FriendAction{Action: "request", Code: code}); r.status != 409 {
		t.Fatal("missing cooldown", r.status)
	}
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "reject-social-a@example.test")
	f.loginAs(t, b, "reject-social-b@example.test")
	socialLink(t, f, a, b)
	create := social.RoomAction{Action: "create", Name: "Лимит", Mode: "pvp", Metric: "manual", Unit: "count", Target: 1, DailyCap: 1, Days: 1, Consent: true}
	room := socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", create)).ID
	socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", social.RoomAction{Action: "invite", ID: room, Recipient: other.ID}))
	socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/rooms", "", social.RoomAction{Action: "reject", ID: room, Version: 1}))
	if rooms := socialRead[[]social.Room](t, f.privateRequest(t, b, "GET", "/social/rooms", "", nil)); len(rooms) != 0 {
		t.Fatal("rejected room visible")
	}
	if r := f.privateRequest(t, b, "POST", "/social/rooms", "", social.RoomAction{Action: "finalize", ID: room}); r.status != 409 {
		t.Fatal("left member finalization", r.status)
	}
	for i := 1; i < 10; i++ {
		socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", create))
	}
	if r := f.privateRequest(t, a, "POST", "/social/rooms", "", create); r.status != 429 {
		t.Fatal("missing creation quota", r.status)
	}
	if r := f.privateRequest(t, a, "POST", "/social/moderation", "", social.ModerationAction{Action: "report", Target: testkit.NewUser().ID, Reason: "spam"}); r.status != 404 {
		t.Fatal("unknown report target", r.status)
	}
}

func TestSocialPermissionAndValidationMatrix(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "matrix-a@example.test")
	b, other := f.signedIn(t, "matrix-b@example.test")
	c, third := f.signedIn(t, "matrix-c@example.test")
	socialVerify(t, f, who.ID)
	socialVerify(t, f, other.ID)
	socialLink(t, f, a, b)
	check := func(client *http.Client, path string, input any, want int) {
		t.Helper()
		r := f.privateRequest(t, client, "POST", "/social/"+path, "", input)
		if r.status != want {
			t.Fatalf("%s want %d got %d %s", path, want, r.status, r.body)
		}
	}
	check(a, "friends", social.FriendAction{Action: "request", Code: "bad"}, 404)
	check(a, "friends", social.FriendAction{Action: "accept", ID: "bad"}, 404)
	check(a, "grants", social.GrantAction{Recipient: who.ID, Metric: "regularity", Allow: true}, 404)
	check(a, "grants", social.GrantAction{Recipient: third.ID, Metric: "regularity", Allow: true}, 404)
	check(a, "grants", social.GrantAction{Recipient: other.ID, Metric: "regularity", Scope: "all", Allow: true}, 422)
	check(a, "grants", social.GrantAction{Recipient: other.ID, Metric: "challenge", Scope: "bad", Allow: true}, 404)
	check(a, "grants", social.GrantAction{Recipient: other.ID, Metric: "challenge", Scope: testkit.NewUser().ID, Allow: true}, 404)
	run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", challengeInput(t, 0)))
	check(a, "grants", social.GrantAction{Recipient: other.ID, Metric: "challenge", Scope: run.ID, Allow: true}, 200)
	check(a, "grants", social.GrantAction{Recipient: other.ID, Metric: "evening", Allow: true}, 200)
	socialRead[[]social.Grant](t, f.privateRequest(t, b, "GET", "/social/grants", "", nil))
	check(a, "grants", social.GrantAction{Recipient: other.ID, Metric: "challenge", Scope: run.ID}, 200)
	create := social.RoomAction{Action: "create", Mode: "coop", Name: "Согласие", Metric: "manual", Unit: "count", Target: 3, DailyCap: 3, Days: 3, Consent: true}
	check(c, "rooms", create, 403)
	bad := create
	bad.Mode = "xp"
	check(a, "rooms", bad, 422)
	bad = create
	bad.Metric = "xp"
	check(a, "rooms", bad, 422)
	bad = create
	bad.Consent = false
	check(a, "rooms", bad, 422)
	id := socialRead[social.Receipt](t, f.privateRequest(t, a, "POST", "/social/rooms", "", create)).ID
	check(c, "rooms", social.RoomAction{Action: "finalize", ID: id}, 404)
	check(a, "rooms", social.RoomAction{Action: "finalize", ID: id}, 409)
	check(a, "rooms", social.RoomAction{Action: "unknown", ID: id}, 422)
	check(a, "rooms", social.RoomAction{Action: "leave", ID: "bad"}, 404)
	check(b, "rooms", social.RoomAction{Action: "invite", ID: id, Recipient: third.ID}, 404)
	check(a, "rooms", social.RoomAction{Action: "invite", ID: id, Recipient: other.ID}, 200)
	check(a, "rooms", social.RoomAction{Action: "invite", ID: id, Recipient: other.ID}, 409)
	list := socialRead[[]social.Room](t, f.privateRequest(t, b, "GET", "/social/rooms", "", nil))
	check(b, "rooms", social.RoomAction{Action: "join", ID: id, Version: 1, RulesHash: "wrong", Consent: true}, 409)
	check(b, "rooms", social.RoomAction{Action: "join", ID: id, Version: 1, RulesHash: list[0].RulesHash, Consent: true}, 200)
	check(b, "rooms", social.RoomAction{Action: "sharing", ID: id, Version: 1, Consent: true}, 409)
	check(b, "rooms", social.RoomAction{Action: "checkin", ID: id, Amount: 4}, 422)
	check(b, "rooms", social.RoomAction{Action: "checkin", ID: id, Amount: 1}, 200)
	f.clock.Advance(time.Second)
	list = socialRead[[]social.Room](t, f.privateRequest(t, b, "GET", "/social/rooms", "", nil))
	checkin := list[0].Checkin.ID
	check(b, "reviews", social.ReviewAction{Action: "request", ID: checkin, Recipient: who.ID}, 422)
	check(b, "reviews", social.ReviewAction{Action: "request", ID: checkin, Recipient: who.ID, Consent: true, AttachmentID: testkit.NewUser().ID}, 404)
	review := socialRead[social.Receipt](t, f.privateRequest(t, b, "POST", "/social/reviews", "", social.ReviewAction{Action: "request", ID: checkin, Recipient: who.ID, Consent: true})).ID
	check(c, "reviews", social.ReviewAction{Action: "accepted", ID: review, Version: 1}, 404)
	check(a, "reviews", social.ReviewAction{Action: "accepted", ID: review, Version: 9}, 409)
	check(a, "reviews", social.ReviewAction{Action: "withdrawn", ID: review, Version: 1}, 404)
	check(a, "reviews", social.ReviewAction{Action: "rejected", ID: review, Version: 1}, 200)
	check(b, "rooms", social.RoomAction{Action: "checkin", ID: id, Amount: 2, Version: 1}, 200)
	check(a, "reviews", social.ReviewAction{Action: "accepted", ID: review, Version: 2}, 409)
	check(b, "reviews", social.ReviewAction{Action: "request", ID: checkin, Recipient: who.ID, Consent: true, Version: 2}, 200)
	check(a, "reviews", social.ReviewAction{Action: "accepted", ID: review, Version: 3}, 200)
	socialRead[[]social.Review](t, f.privateRequest(t, a, "GET", "/social/reviews", "", nil))
	check(b, "rooms", social.RoomAction{Action: "sharing", ID: id, Version: 2}, 200)
	reviews := socialRead[[]social.Review](t, f.privateRequest(t, a, "GET", "/social/reviews", "", nil))
	if len(reviews) != 0 {
		t.Fatal("revoked review visible")
	}
	check(a, "community", social.CommunityAction{Action: "preferences", Enabled: true, Version: 1}, 422)
	check(a, "community", social.CommunityAction{Action: "preferences", Recipient: other.ID, Version: 99}, 409)
	check(a, "community", social.CommunityAction{Action: "nudge", Recipient: other.ID, Template: "spam"}, 422)
	check(a, "community", social.CommunityAction{Action: "publish", Consent: true, SourceKey: "invented", SourceVersion: "x"}, 404)
	check(a, "community", social.CommunityAction{Action: "unknown"}, 422)
	check(a, "moderation", social.ModerationAction{Action: "block", Target: who.ID}, 404)
	check(a, "moderation", social.ModerationAction{Action: "block", Target: third.ID}, 404)
	check(a, "moderation", social.ModerationAction{Action: "report", Target: other.ID, Reason: "unknown"}, 422)
	check(a, "moderation", social.ModerationAction{Action: "hide", ID: "bad"}, 404)
	check(a, "moderation", social.ModerationAction{Action: "report", ID: "bad", Reason: "spam"}, 404)
	check(a, "moderation", social.ModerationAction{Action: "report", ID: testkit.NewUser().ID, Reason: "spam"}, 404)
	check(a, "moderation", social.ModerationAction{Action: "resolve", ID: review}, 404)
	check(a, "moderation", social.ModerationAction{Action: "unknown"}, 422)
	// Revoking a friendship also invalidates queued room invitation sources.
	check(b, "moderation", social.ModerationAction{Action: "block", Target: who.ID}, 200)
	check(a, "rooms", social.RoomAction{Action: "invite", ID: id, Recipient: other.ID}, 404)
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM notification_invite_sources WHERE recipient_id=$1 AND active", other.ID).Scan(&n); e != nil || n != 0 {
		t.Fatal("blocked invitation job still eligible", n, e)
	}
	socialRead[social.Dashboard](t, f.privateRequest(t, a, "GET", "/social/dashboard", "", nil))
}
