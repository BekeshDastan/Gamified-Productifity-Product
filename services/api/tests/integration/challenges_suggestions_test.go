//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/challenges"
	"sync"
	"testing"
)

func TestChallengeSuggestionsRequireAcceptanceAndDeduplicate(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "quest-suggestions@example.test")
	var list struct{ Data challenges.Suggestions }
	r := f.privateRequest(t, a, "GET", "/challenges/suggestions?period=daily&available_minutes=5", "", nil)
	if r.status != 200 || json.Unmarshal(r.body, &list) != nil || len(list.Data.Items) == 0 {
		t.Fatal(r.status, string(r.body))
	}
	for _, v := range list.Data.Items {
		if v.Template.EstimatedMinutes > 5 || v.QuestID != nil {
			t.Fatal(v)
		}
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM quests WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 0 {
		t.Fatal("unaccepted obligation", n, e)
	}
	in := challenges.AcceptSuggestion{Period: "daily", StartsOn: list.Data.StartsOn, TemplateID: list.Data.Items[0].Template.ID, AvailableMinutes: 5}
	results := make([]authResponse, 4)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = f.privateRequest(t, a, "POST", "/challenges/suggestions/accept", "", in)
		}(i)
	}
	wg.Wait()
	id := ""
	for _, r := range results {
		var out struct{ Data challenges.AcceptedSuggestion }
		if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
			t.Fatal(r.status, string(r.body))
		}
		if id != "" && id != out.Data.QuestID {
			t.Fatal("duplicate quests")
		}
		id = out.Data.QuestID
	}
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM quests WHERE owner_id=$1 AND status='backlog' AND planned_date IS NULL", who.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal("acceptance changed plan", n, e)
	}
	in.AvailableMinutes = 10
	if r := f.privateRequest(t, a, "POST", "/challenges/suggestions/accept", "", in); r.status != 409 {
		t.Fatal("changed request", r.status)
	}
	if r := f.privateRequest(t, a, "GET", "/challenges/suggestions?period=daily&available_minutes=0", "", nil); r.status != 422 {
		t.Fatal(r.status)
	}
}
