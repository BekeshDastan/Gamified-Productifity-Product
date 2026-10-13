//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/goals"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/journal"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"strings"
	"testing"
	"time"
)

func TestJournalCursorIdempotencyAndPrivateProjectionPayloads(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "journal-pages@example.test")
	goal := readGoal(t, f.privateRequest(t, a, "POST", "/goals", "", goals.Create{Input: goalInput("Моя цель"), State: "active"}))
	input := journal.Input{Date: "2026-10-12", Title: "Первая запись", Body: "JOURNAL_PRIVATE_SENTINEL_C120", GoalID: &goal.ID, Tags: []string{"вывод"}}
	key := testkit.NewUser().ID
	first := readEntry(t, f.privateRequest(t, a, "POST", "/journal/entries", "", input, key))
	replay := readEntry(t, f.privateRequest(t, a, "POST", "/journal/entries", "", input, key))
	if replay.ID != first.ID {
		t.Fatal("duplicate create")
	}
	input.Title = "Другая запись"
	if r := f.privateRequest(t, a, "POST", "/journal/entries", "", input, key); r.status != 409 {
		t.Fatal("key mismatch", r.status)
	}
	for i := 0; i < 25; i++ {
		input.Title = fmt.Sprintf("Запись %d", i)
		readEntry(t, f.privateRequest(t, a, "POST", "/journal/entries", "", input))
	}
	page := func(path string) journal.Page {
		r := f.privateRequest(t, a, "GET", path, "", nil)
		var out struct{ Data journal.Page }
		if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
			t.Fatal(r.status, string(r.body))
		}
		return out.Data
	}
	one := page("/journal/entries?tag=вывод")
	two := page("/journal/entries?tag=вывод&after=" + one.Next)
	if len(one.Items) != 25 || one.Next == "" || len(two.Items) != 1 || two.Next != "" {
		t.Fatal(len(one.Items), len(two.Items))
	}
	seen := map[string]bool{}
	for _, item := range append(one.Items, two.Items...) {
		if seen[item.ID] {
			t.Fatal("duplicate cursor")
		}
		seen[item.ID] = true
	}
	if len(page("/journal/entries?from=2026-10-13&to=2026-10-14").Items) != 0 {
		t.Fatal("date filter")
	}
	for _, path := range []string{"/journal/entries?from=bad", "/journal/entries?from=2026-10-14&to=2026-10-12", "/journal/entries?tag=" + strings.Repeat("a", 51), "/routine-runs?before=-1"} {
		if r := f.privateRequest(t, a, "GET", path, "", nil); r.status != 422 {
			t.Fatal(path, r.status)
		}
	}
	for _, path := range []string{"/journal/entries/bad", "/journal/entries/" + testkit.NewUser().ID, "/routine-runs/templates/bad"} {
		if r := f.privateRequest(t, a, "GET", path, "", nil); r.status != 404 {
			t.Fatal(path, r.status)
		}
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM outbox_events WHERE owner_id=$1 AND payload::text LIKE '%JOURNAL_PRIVATE_SENTINEL_C120%'", who.ID).Scan(&n); e != nil || n != 0 {
		t.Fatal("private text in projection", n, e)
	}
	if r := f.privateRequest(t, a, "POST", "/journal/entries", "", map[string]any{"secret_field": "unexpected"}); r.status != 422 {
		t.Fatal("strict DTO", r.status)
	}
}
func TestJournalUnplannedCompletionAndFrozenReviewTimezone(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "journal-zone@example.test")
	if _, e := f.pool.Exec(f.ctx, `UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}','"Asia/Almaty"') WHERE owner_id=$1`, who.ID); e != nil {
		t.Fatal(e)
	}
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Незапланированный результат")))
	f.clock.Advance(10*time.Hour + 30*time.Minute)
	f.loginAs(t, a, "journal-zone@example.test")
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), map[string]string{"reason": "Получилось"}))
	current := readReview(t, f.privateRequest(t, a, "GET", "/reviews/day/2026-10-13", "", nil))
	if current.Stats.Planned != 0 || current.Stats.Completed != 0 || current.Stats.CompletedToday != 1 || len(current.Stats.Items) != 1 || current.Stats.Items[0].Planned {
		t.Fatal(current)
	}
	prior := readReview(t, f.privateRequest(t, a, "GET", "/reviews/day/2026-10-12", "", nil))
	if prior.Stats.CompletedToday != 0 {
		t.Fatal(prior)
	}
	saved := readReview(t, f.privateRequest(t, a, "PUT", "/reviews/day/2026-10-13", `"0"`, journal.ReviewInput{Obstacle: "none", Conclusion: "Незапланированный шаг тоже имеет источник"}))
	if saved.Timezone != "Asia/Almaty" {
		t.Fatal(saved)
	}
	if _, e := f.pool.Exec(f.ctx, `UPDATE user_settings SET preferences=jsonb_set(preferences,'{timezone}','"UTC"') WHERE owner_id=$1`, who.ID); e != nil {
		t.Fatal(e)
	}
	restored := readReview(t, f.privateRequest(t, a, "GET", "/reviews/day/2026-10-13", "", nil))
	if restored.Timezone != "Asia/Almaty" || restored.Stats.CompletedToday != 1 || restored.Run.ID != saved.Run.ID {
		t.Fatal("timezone rewrote saved review", restored)
	}
}
