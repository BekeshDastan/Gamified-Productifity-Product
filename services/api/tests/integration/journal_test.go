//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/journal"
	"strings"
	"testing"
)

func readEntry(t *testing.T, r authResponse) journal.Entry {
	t.Helper()
	var out struct{ Data journal.Entry }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestJournalPrivacyValidationVersionsAndRendering(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "journal@example.test")
	b, other := f.signedIn(t, "journal-other@example.test")
	in := journal.Input{Date: "2026-10-12", Title: "Личный вывод", Body: "# Итог\n<script>secret</script>\n[bad](javascript:alert)", Tags: []string{"работа"}}
	created := readEntry(t, f.privateRequest(t, a, "POST", "/journal/entries", "", in))
	if strings.Contains(created.HTML, "<script") || strings.Contains(created.HTML, `href="javascript:`) {
		t.Fatal(created.HTML)
	}
	if r := f.privateRequest(t, b, "GET", "/journal/entries/"+created.ID, "", nil); r.status != 404 {
		t.Fatal("foreign read", r.status)
	}
	if r := f.privateRequest(t, b, "PUT", "/journal/entries/"+created.ID, `"1"`, in); r.status != 404 {
		t.Fatal("foreign edit", r.status)
	}
	// Even an administrator must use their own owner scope.
	if _, e := f.pool.Exec(f.ctx, "UPDATE users SET role='admin' WHERE id=$1", other.ID); e != nil {
		t.Fatal(e)
	}
	if r := f.privateRequest(t, b, "GET", "/journal/entries/"+created.ID, "", nil); r.status != 404 {
		t.Fatal("administrator private read", r.status)
	}
	in.Title = "Исправленный вывод"
	updated := readEntry(t, f.privateRequest(t, a, "PUT", "/journal/entries/"+created.ID, `"1"`, in))
	if updated.Version != 2 {
		t.Fatal(updated)
	}
	if r := f.privateRequest(t, a, "PUT", "/journal/entries/"+created.ID, `"1"`, in); r.status != 409 {
		t.Fatal("stale", r.status)
	}
	in.Body = strings.Repeat("я", 50001)
	if r := f.privateRequest(t, a, "POST", "/journal/entries", "", in); r.status != 422 {
		t.Fatal("long body", r.status)
	}
	in.Body = ""
	in.Date = "2026-02-30"
	if r := f.privateRequest(t, a, "POST", "/journal/entries", "", in); r.status != 422 {
		t.Fatal("date", r.status)
	}
	in.Date = "2026-10-12"
	in.Tags = []string{"a", "a"}
	if r := f.privateRequest(t, a, "POST", "/journal/entries", "", in); r.status != 422 {
		t.Fatal("tags", r.status)
	}
	in.Tags = []string{}
	q := readQuest(t, f.privateRequest(t, b, "POST", "/quests", "", questInput("чужая")))
	in.GoalID = &q.ID
	if r := f.privateRequest(t, a, "POST", "/journal/entries", "", in); r.status != 404 {
		t.Fatal("goal", r.status)
	}
	in.GoalID = nil
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM journal_entries WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	in.Archived = true
	readEntry(t, f.privateRequest(t, a, "PUT", "/journal/entries/"+created.ID, fmt.Sprintf(`"%d"`, updated.Version), in))
	for _, path := range []string{"/journal/entries", "/journal/entries?tag=работа", "/journal/entries?archived=true", "/journal/entries?from=2026-10-12&to=2026-10-12"} {
		r := f.privateRequest(t, a, "GET", path, "", nil)
		if r.status != 200 {
			t.Fatal(path, r.status, string(r.body))
		}
	}
	if r := f.privateRequest(t, a, "GET", "/journal/entries?after=invalid", "", nil); r.status != 422 {
		t.Fatal("cursor", r.status)
	}
}
