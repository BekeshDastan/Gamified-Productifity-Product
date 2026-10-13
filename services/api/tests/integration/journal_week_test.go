//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/journal"
	"testing"
	"time"
)

func readWeek(t *testing.T, r authResponse) journal.Week {
	t.Helper()
	var out struct{ Data journal.Week }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func readWeekPreview(t *testing.T, r authResponse) journal.WeekPreview {
	t.Helper()
	var out struct{ Data journal.WeekPreview }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestJournalWeeklyPreviewKeepsUnfinishedDatesAndMakesNoChanges(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "week@example.test")
	b, _ := f.signedIn(t, "week-other@example.test")
	date := "2026-10-05"
	input := questInput("Старая незавершённая задача")
	input.PlannedDate = &date
	deadline := f.clock.Now().Add(-24 * time.Hour)
	input.Deadline = &deadline
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", input))
	path := "/reviews/week/2026-10-12"
	week := readWeek(t, f.privateRequest(t, a, "GET", path, "", nil))
	if len(week.Snapshot.Items) != 1 || week.Snapshot.Items[0].PlannedDate == nil || *week.Snapshot.Items[0].PlannedDate != date || len(week.Snapshot.Plans) != 7 {
		t.Fatal(week)
	}
	preview := readWeekPreview(t, f.privateRequest(t, a, "POST", path+"/preview", "", struct{}{}))
	if preview.ID == "" || !preview.ExpiresAt.Equal(f.clock.Now().Add(10*time.Minute)) {
		t.Fatal(preview)
	}
	unchanged := readQuest(t, f.privateRequest(t, a, "GET", "/quests/"+q.ID, "", nil))
	if unchanged.Version != q.Version || *unchanged.PlannedDate != date || !unchanged.Deadline.Equal(deadline) {
		t.Fatal("preview mutated task", unchanged)
	}
	if w := readWeek(t, f.privateRequest(t, b, "GET", path, "", nil)); len(w.Snapshot.Items) != 0 {
		t.Fatal("privacy")
	}
	if r := f.privateRequest(t, a, "GET", "/reviews/week/2026-10-13", "", nil); r.status != 422 {
		t.Fatal("week start", r.status)
	}
	if _, e := f.pool.Exec(f.ctx, "UPDATE weekly_review_previews SET snapshot='{}' WHERE owner_id=$1", who.ID); e == nil {
		t.Fatal("mutable snapshot")
	}
}
