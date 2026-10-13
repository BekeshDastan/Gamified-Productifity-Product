//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/journal"
	"testing"
)

func readReview(t *testing.T, r authResponse) journal.Review {
	t.Helper()
	var out struct{ Data journal.Review }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestJournalEveningActualSourcesNullableMoodAndOneReview(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "review@example.test")
	b, _ := f.signedIn(t, "review-other@example.test")
	date := "2026-10-12"
	inQ := questInput("Сделанный шаг")
	inQ.PlannedDate = &date
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", inQ))
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", fmt.Sprintf(`"%d"`, q.Version), map[string]string{"reason": "Готово"}))
	path := "/reviews/day/" + date
	review := readReview(t, f.privateRequest(t, a, "GET", path, "", nil))
	if review.Run != nil || review.Stats.Planned != 1 || review.Stats.Completed != 1 || len(review.Stats.Items) != 1 {
		t.Fatal(review)
	}
	input := journal.ReviewInput{Obstacle: "none", Conclusion: "Один шаг уже сделан"}
	saved := readReview(t, f.privateRequest(t, a, "PUT", path, `"0"`, input))
	if saved.Run == nil || saved.Run.Version != 1 {
		t.Fatal(saved)
	}
	input.Conclusion = "Уточнённый вывод"
	again := readReview(t, f.privateRequest(t, a, "PUT", path, `"1"`, input))
	if again.Run.ID != saved.Run.ID || again.Run.Version != 2 {
		t.Fatal(again)
	}
	if r := f.privateRequest(t, a, "PUT", path, `"1"`, input); r.status != 409 {
		t.Fatal("stale", r.status)
	}
	empty := readReview(t, f.privateRequest(t, b, "GET", path, "", nil))
	if empty.Run != nil || empty.Stats.Completed != 0 {
		t.Fatal("privacy", empty)
	}
	v := 6
	input.Energy = &v
	if r := f.privateRequest(t, a, "PUT", path, `"2"`, input); r.status != 422 {
		t.Fatal("energy", r.status)
	}
	if r := f.privateRequest(t, a, "GET", "/reviews/day/2026-10-14", "", nil); r.status != 422 {
		t.Fatal("future", r.status)
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM daily_reviews WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if _, e := f.pool.Exec(f.ctx, "UPDATE routine_run_events SET snapshot='{}' WHERE owner_id=$1", who.ID); e == nil {
		t.Fatal("history changed")
	}
}
