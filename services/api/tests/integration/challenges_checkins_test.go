//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"testing"
	"time"
)

func readCheckin(t *testing.T, r authResponse) challenges.Checkin {
	t.Helper()
	var out struct{ Data challenges.Checkin }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func readEvidence(t *testing.T, r authResponse) challenges.Evidence {
	t.Helper()
	var out struct{ Data challenges.Evidence }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestChallengeManualCheckinsCorrectionsPermanentReplayAndPrivateEvidence(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits(), cleanFixtureScanner{})
	a, who := f.signedIn(t, "manual-challenge@example.test")
	b, _ := f.signedIn(t, "manual-challenge-other@example.test")
	run := readChallenge(t, f.privateRequest(t, a, "POST", "/challenges/runs", "", challengeInput(t, 8)))
	path := "/challenges/runs/" + run.ID
	input := challenges.CheckinInput{OperationID: testkit.NewUser().ID, Unit: "pages", Amount: 10, Note: "Частная заметка"}
	if r := f.privateRequest(t, a, "POST", path+"/check-ins", "", input); r.status != 409 {
		t.Fatal("before join", r.status)
	}
	readChallengeMember(t, f.privateRequest(t, a, "POST", path+"/join", "", challenges.JoinInput{RulesHash: run.RulesHash, AcceptRules: true}))
	f.clock.Advance(time.Minute)
	v := readCheckin(t, f.privateRequest(t, a, "POST", path+"/check-ins", "", input))
	if v.Confirmation != "self_report" || v.Amount != 10 || v.InitialAmount != 10 {
		t.Fatal(v)
	}
	check := func(want float64) {
		t.Helper()
		p := challengeProgress(t, readChallengeMember(t, f.privateRequest(t, a, "GET", path+"/progress", "", nil)))
		if p.Current != want {
			t.Fatal(p, want)
		}
	}
	check(10)
	if again := readCheckin(t, f.privateRequest(t, a, "POST", path+"/check-ins", "", input)); again.ID != v.ID {
		t.Fatal("duplicate")
	}
	modified := input
	modified.Amount = 11
	if r := f.privateRequest(t, a, "POST", path+"/check-ins", "", modified); r.status != 409 {
		t.Fatal("changed operation", r.status)
	}
	correction := challenges.CheckinCorrection{Amount: 0, Note: "Ошибка"}
	v = readCheckin(t, f.privateRequest(t, a, "PATCH", path+"/check-ins/"+v.ID, `"1"`, correction))
	check(0)
	if r := f.privateRequest(t, a, "PATCH", path+"/check-ins/"+v.ID, `"1"`, correction); r.status != 409 {
		t.Fatal("stale", r.status)
	}
	correction.Amount = 11
	if r := f.privateRequest(t, a, "PATCH", path+"/check-ins/"+v.ID, fmt.Sprintf(`"%d"`, v.Version), correction); r.status != 422 {
		t.Fatal("inflated correction", r.status)
	}
	correction.Amount = 10
	v = readCheckin(t, f.privateRequest(t, a, "PATCH", path+"/check-ins/"+v.ID, fmt.Sprintf(`"%d"`, v.Version), correction))
	check(10)
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Личное доказательство")))
	file := readAttachment(t, f.uploadFile(t, a, q, "evidence.txt", []byte("private evidence")))
	ep := path + "/check-ins/" + v.ID + "/evidence/" + file.ID
	ev := readEvidence(t, f.privateRequest(t, a, "PUT", ep, `"0"`, challenges.EvidenceInput{Visibility: "private"}))
	if ev.Visibility != "private" || ev.ConsentAt != nil || ev.Confirmation != "self_report" {
		t.Fatal(ev)
	}
	if r := f.privateRequest(t, b, "GET", ev.URL[len("/api/v1"):], "", nil); r.status != 404 {
		t.Fatal("foreign file download", r.status)
	}
	if r := f.privateRequest(t, a, "PUT", ep, `"1"`, challenges.EvidenceInput{Visibility: "participants"}); r.status != 422 {
		t.Fatal("missing consent", r.status)
	}
	ev = readEvidence(t, f.privateRequest(t, a, "PUT", ep, `"1"`, challenges.EvidenceInput{Visibility: "participants", Consent: true}))
	if ev.ConsentAt == nil || ev.Version != 2 {
		t.Fatal(ev)
	}
	if r := f.privateRequest(t, b, "GET", ev.URL[len("/api/v1"):], "", nil); r.status != 404 {
		t.Fatal("consent expanded to strangers", r.status)
	}
	ev = readEvidence(t, f.privateRequest(t, a, "PUT", ep, `"2"`, challenges.EvidenceInput{Visibility: "private"}))
	if ev.ConsentAt != nil || ev.Version != 3 {
		t.Fatal(ev)
	}
	if repeat := readEvidence(t, f.privateRequest(t, a, "PUT", ep, `"3"`, challenges.EvidenceInput{Visibility: "private"})); repeat.Version != 3 {
		t.Fatal("same consent rewrote history")
	}
	var evidenceList struct{ Data []challenges.Evidence }
	er := f.privateRequest(t, a, "GET", path+"/check-ins/"+v.ID+"/evidence", "", nil)
	if er.status != 200 || json.Unmarshal(er.body, &evidenceList) != nil || len(evidenceList.Data) != 1 || !evidenceList.Data[0].Available {
		t.Fatal(er.status, string(er.body))
	}
	if r := f.privateRequest(t, a, "PUT", ep, `"2"`, challenges.EvidenceInput{Visibility: "participants", Consent: true}); r.status != 409 {
		t.Fatal("stale consent", r.status)
	}
	if r := f.privateRequest(t, a, "DELETE", "/attachments/"+file.ID, fmt.Sprintf(`"%d"`, file.Version), struct{}{}); r.status != 200 {
		t.Fatal("delete evidence file", r.status, string(r.body))
	}
	er = f.privateRequest(t, a, "GET", path+"/check-ins/"+v.ID+"/evidence", "", nil)
	if er.status != 200 || json.Unmarshal(er.body, &evidenceList) != nil || evidenceList.Data[0].Available || evidenceList.Data[0].URL != "" || evidenceList.Data[0].Name != "Файл недоступен" {
		t.Fatal("deleted evidence still downloadable", er.status, string(er.body))
	}
	if r := f.privateRequest(t, b, "GET", path+"/check-ins/"+v.ID+"/evidence", "", nil); r.status != 404 {
		t.Fatal("foreign evidence", r.status)
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM challenge_evidence_events WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 3 {
		t.Fatal(n, e)
	}
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "manual-challenge@example.test")
	if again := readCheckin(t, f.privateRequest(t, a, "POST", path+"/check-ins", "", input)); again.ID != v.ID || again.Version != v.Version {
		t.Fatal("permanent replay")
	}
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM reward_entitlements WHERE owner_id=$1 AND source_kind='challenge'", who.ID).Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
}
