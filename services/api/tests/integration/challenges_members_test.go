//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"sync"
	"testing"
	"time"
)

func readChallengeMember(t *testing.T, r authResponse) challenges.Member {
	t.Helper()
	var out struct{ Data challenges.Member }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestChallengeMembershipRaceLeaveRejoinAndPermanentIdentity(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "challenge-member@example.test")
	b, _ := f.signedIn(t, "challenge-member-other@example.test")
	input := challengeInput(t, 0)
	input.Input.Rejoin = true
	run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", input))
	path := "/challenges/runs/" + run.ID
	join := challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}
	results := make([]authResponse, 4)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i] = f.privateRequest(t, a, "POST", path+"/join", "", join) }(i)
	}
	wg.Wait()
	member := readChallengeMember(t, results[0])
	for _, r := range results {
		if m := readChallengeMember(t, r); m.ID != member.ID || m.Version != 1 {
			t.Fatal("duplicate member")
		}
	}
	if r := f.privateRequest(t, b, "POST", path+"/join", "", join); r.status != 404 {
		t.Fatal("foreign join", r.status)
	}
	if r := f.privateRequest(t, a, "POST", path+"/cancel", `"1"`, challenges.LeaveInput{}); r.status != 409 {
		t.Fatal("cancel active", r.status)
	}
	f.clock.Advance(time.Minute)
	left := readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/leave", `"1"`, challenges.LeaveInput{}))
	if left.State != "left" || left.LeftAt == nil {
		t.Fatal(left)
	}
	repeat := readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/leave", `"1"`, challenges.LeaveInput{}))
	if repeat.Version != left.Version {
		t.Fatal("duplicate leave")
	}
	if r := f.privateRequest(t, a, "POST", path+"/join", "", join); r.status != 409 {
		t.Fatal("stale original join reactivated", r.status)
	}
	f.clock.Advance(time.Minute)
	join.MembershipVersion = left.Version
	rejoined := readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/join", "", join))
	if rejoined.ID != member.ID || rejoined.Version != 3 || !rejoined.FirstJoinedAt.Equal(member.FirstJoinedAt) {
		t.Fatal(rejoined)
	}
	if r := f.privateRequest(t, a, "POST", path+"/leave", `"1"`, challenges.LeaveInput{}); r.status != 409 {
		t.Fatal("old leave cancelled new window", r.status)
	}
	var n int
	for query, want := range map[string]int{"SELECT count(*) FROM challenge_participants WHERE owner_id=$1": 1, "SELECT count(*) FROM challenge_membership_windows WHERE owner_id=$1": 2, "SELECT count(*) FROM challenge_membership_windows WHERE owner_id=$1 AND left_at IS NULL": 1, "SELECT count(*) FROM challenge_participant_events WHERE owner_id=$1": 3} {
		if e := f.pool.QueryRow(f.ctx, query, who.ID).Scan(&n); e != nil || n != want {
			t.Fatal(n, e)
		}
	}
	if _, e := f.pool.Exec(f.ctx, "UPDATE challenge_membership_windows SET joined_at=joined_at+interval '1 second' WHERE owner_id=$1 AND left_at IS NOT NULL", who.ID); e == nil {
		t.Fatal("closed window mutable")
	}
	readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/leave", fmt.Sprintf(`"%d"`, rejoined.Version), challenges.LeaveInput{Reason: "Личная причина"}))
	closed := readChallenge(t, f.privateRequest(t, a, "POST", path+"/cancel", `"1"`, challenges.LeaveInput{Reason: "Выбрал другой вариант"}))
	if closed.State != "closed" || closed.FinalSnapshot == nil {
		t.Fatal(closed)
	}
	var membership struct{ Data challenges.Membership }
	r := f.privateRequest(t, a, "GET", path+"/membership", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &membership) != nil || membership.Data.Member == nil || membership.Data.Member.State != "finished" || membership.Data.Member.Result == nil || *membership.Data.Member.Result != "left" {
		t.Fatal(r.status, string(r.body))
	}
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM reward_ledger WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 0 {
		t.Fatal("join/leave awarded XP", n, e)
	}
}
func TestChallengeJoinRequiresConsentAndHonorsNoRejoinAndExpiry(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "challenge-no-rejoin@example.test")
	run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", challengeInput(t, 0)))
	path := "/challenges/runs/" + run.ID
	for _, in := range []challenges.JoinInput{{RulesHash: run.RulesHash}, {RulesHash: "wrong", AcceptRules: true}} {
		if r := f.privateRequest(t, a, "POST", path+"/join", "", in); r.status != 422 {
			t.Fatal("consent", r.status)
		}
	}
	member := readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}))
	left := readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/leave", fmt.Sprintf(`"%d"`, member.Version), challenges.LeaveInput{}))
	if r := f.privateRequest(t, a, "POST", path+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true, MembershipVersion: left.Version}); r.status != 409 {
		t.Fatal("rejoin forbidden", r.status)
	}
	fresh := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", challengeInput(t, 0)))
	f.clock.Advance(7 * 24 * time.Hour)
	f.loginAs(t, a, "challenge-no-rejoin@example.test")
	if r := f.privateRequest(t, a, "POST", "/challenges/runs/"+fresh.ID+"/join", "", challenges.JoinInput{RulesHash: fresh.RulesHash, AcceptRules: true}); r.status != 409 {
		t.Fatal("expired join", r.status)
	}
}
