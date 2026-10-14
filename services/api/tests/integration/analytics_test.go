//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/jackc/pgx/v5"
	"testing"
)

func readAnalytics(t *testing.T, r authResponse) analytics.View {
	t.Helper()
	var out struct{ Data analytics.View }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestAnalyticsProjectionRebuildCorrectionsAndDuplicateEvent(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, who := f.signedIn(t, "analytics-source@example.test")
	b, _ := f.signedIn(t, "analytics-private@example.test")
	rangeIn := analytics.Range{From: "2026-10-12", To: "2026-10-19"}
	path := "/analytics?from=" + rangeIn.From + "&to=" + rangeIn.To
	empty := readAnalytics(t, f.privateRequest(t, a, "GET", path, "", nil))
	if len(empty.Days) != 7 || len(empty.Weeks) != 1 || empty.Weeks[0].Days != 7 || empty.Stale || empty.ComputedAt == nil || empty.Days[0].Completed != 0 {
		t.Fatal(empty)
	}
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Источник аналитики")))
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	delayed := readAnalytics(t, f.privateRequest(t, a, "GET", path, "", nil))
	if !delayed.Stale || delayed.Days[0].Completed != 0 || delayed.SourceRevision <= delayed.ProjectedRevision {
		t.Fatal("stale projection presented as fresh", delayed)
	}
	current := readAnalytics(t, f.privateRequest(t, a, "POST", "/analytics/rebuild", "", rangeIn))
	if current.Stale || current.Days[0].Completed != 1 || current.Days[0].XPDelta != 10 || current.Weeks[0].Completed != 1 || len(current.Days[0].Sources) != 3 {
		t.Fatal(current)
	}
	var taskSource bool
	for _, s := range current.Days[0].Sources {
		if s.Kind == "task" && s.ID == q.ID && s.URL == "/app/quests/"+q.ID {
			taskSource = true
		}
	}
	if !taskSource {
		t.Fatal("no exact source")
	}
	other := readAnalytics(t, f.privateRequest(t, b, "GET", path, "", nil))
	if other.Days[0].Completed != 0 || len(other.Days[0].Sources) != 0 {
		t.Fatal("private source leak", other)
	}
	service := analytics.NewService(f.pool, f.clock)
	consume := func() error {
		return storage.WithinTx(f.ctx, f.pool, func(tx pgx.Tx) error {
			return service.Handlers()["analytics.changed"](f.ctx, tx, jobs.Job{OwnerID: &who.ID, Payload: []byte(`{}`)})
		})
	}
	for range 2 {
		if e := consume(); e != nil {
			t.Fatal(e)
		}
	}
	duplicate := readAnalytics(t, f.privateRequest(t, a, "GET", path, "", nil))
	if duplicate.Days[0].Completed != 1 || duplicate.Days[0].XPDelta != 10 {
		t.Fatal("duplicate event increments projection", duplicate)
	}
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/reopen", version(q), quests.Transition{Reason: "Исправление результата"}))
	if e := consume(); e != nil {
		t.Fatal(e)
	}
	corrected := readAnalytics(t, f.privateRequest(t, a, "GET", path, "", nil))
	if corrected.Days[0].Completed != 0 || corrected.Days[0].XPDelta != 0 || corrected.Stale {
		t.Fatal("correction not rebuilt", corrected)
	}
	if _, e := f.pool.Exec(f.ctx, `UPDATE daily_metrics SET payload=jsonb_set(payload,'{completed}','999') WHERE owner_id=$1`, who.ID); e != nil {
		t.Fatal(e)
	}
	rebuilt, e := service.RebuildOwner(f.ctx, who.ID, rangeIn)
	if e != nil || rebuilt.Days[0].Completed != 0 {
		t.Fatal("rebuild trusts counters", rebuilt, e)
	}
	for _, bad := range []string{"/analytics?from=bad&to=2026-10-19", "/analytics?from=2026-10-12&to=2026-10-12", "/analytics?from=2025-01-01&to=2026-10-12"} {
		if r := f.privateRequest(t, a, "GET", bad, "", nil); r.status != 422 {
			t.Fatal("unbounded range", r.status)
		}
	}
	if e := storage.WithinTx(f.ctx, f.pool, func(tx pgx.Tx) error {
		return service.Handlers()["analytics.changed"](f.ctx, tx, jobs.Job{OwnerID: &who.ID, Payload: []byte(`{"code":"never execute"}`)})
	}); e != jobs.ErrPermanent {
		t.Fatal("unexpected event accepted", e)
	}
}
