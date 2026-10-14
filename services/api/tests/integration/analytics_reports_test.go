//go:build integration

package integration

import (
	"encoding/json"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/analytics"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/character"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/notifications"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/progression"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/quests"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/tests/testkit"
	"strings"
	"sync"
	"testing"
	"time"
)

func readReportPreview(t *testing.T, r authResponse) analytics.ReportPreview {
	t.Helper()
	var out struct{ Data analytics.ReportPreview }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func readReport(t *testing.T, r authResponse) analytics.Report {
	t.Helper()
	var out struct{ Data analytics.Report }
	if r.status != 200 || json.Unmarshal(r.body, &out) != nil {
		t.Fatal(r.status, string(r.body))
	}
	return out.Data
}
func TestAnalyticsReportSnapshotZeroBaseRaceCorrectionAndPermanentReplay(t *testing.T) {
	limits := auth.DefaultLimits()
	limits.UserRequests = 600
	f := newAuthFixture(t, limits)
	a, who := f.signedIn(t, "report-snapshot@example.test")
	b, _ := f.signedIn(t, "report-other@example.test")
	f.clock.Advance(14 * 24 * time.Hour)
	f.loginAs(t, a, "report-snapshot@example.test")
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Результат недели")))
	date := "2026-10-26"
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/postpone", version(q), quests.Transition{Reason: "Недостаточно времени", PlannedDate: &date}))
	date = "2026-10-27"
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/postpone", version(q), quests.Transition{Reason: "Недостаточно времени", PlannedDate: &date}))
	q = readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/complete", version(q), quests.Transition{}))
	f.clock.Advance(7 * 24 * time.Hour)
	f.loginAs(t, a, "report-snapshot@example.test")
	f.loginAs(t, b, "report-other@example.test")
	input := analytics.ReportInput{Kind: "week", StartsOn: "2026-10-26", Timezone: "UTC"}
	preview := readReportPreview(t, f.privateRequest(t, a, "POST", "/analytics/reports/preview", "", input))
	snapshot := preview.Snapshot
	if snapshot.FrequentReason == nil || snapshot.FrequentReason.Count != 2 || len(snapshot.FrequentReason.Sources) != 2 || snapshot.FrequentReason.Sources[0].ID != q.ID || !strings.Contains(snapshot.Text, "Недостаточно времени") || snapshot.NextStep == "" || snapshot.Obligations.Planned != 1 {
		t.Fatal("missing report template evidence", snapshot)
	}
	if !snapshot.CurrentObserved || !snapshot.PreviousObserved || snapshot.Comparisons[0].Current != 1 || snapshot.Comparisons[0].Previous == nil || *snapshot.Comparisons[0].Previous != 0 || snapshot.Comparisons[0].Percent != nil || *snapshot.Comparisons[0].Delta != 1 || len(snapshot.Records) != 4 {
		t.Fatal(snapshot)
	}
	in := analytics.SaveReportInput{OperationID: testkit.NewUser().ID, PreviewID: preview.ID, Confirm: true}
	responses := make([]authResponse, 4)
	var wg sync.WaitGroup
	for i := range responses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			responses[i] = f.privateRequest(t, a, "POST", "/analytics/reports", "", in)
		}(i)
	}
	wg.Wait()
	saved := readReport(t, responses[0])
	page := readNotifications(t, f.privateRequest(t, a, "GET", "/notifications", "", nil))
	reportNotifications := 0
	for _, n := range page.Items {
		if n.Kind == "report" && n.SourceID == saved.ID {
			reportNotifications++
			if n.URL != "/app/analytics?report_id="+saved.ID {
				t.Fatal("report link does not select saved snapshot")
			}
			if r := f.privateRequest(t, a, "GET", "/notifications/"+n.ID+"/source", "", nil); r.status != 200 || !strings.Contains(string(r.body), `"available":true`) {
				t.Fatal("current report source", r.status, string(r.body))
			}
			if strings.Contains(n.Message, "Недостаточно времени") {
				t.Fatal("private report reason leaked")
			}
		}
	}
	if reportNotifications != 1 {
		t.Fatal("report callback dedupe", reportNotifications)
	}
	for _, r := range responses {
		if readReport(t, r).ID != saved.ID {
			t.Fatal("duplicate report")
		}
	}
	original, _ := json.Marshal(saved.Snapshot)
	if r := f.privateRequest(t, b, "GET", "/analytics/reports/"+saved.ID, "", nil); r.status != 404 {
		t.Fatal("foreign report", r.status)
	}
	if r := f.privateRequest(t, b, "POST", "/analytics/reports", "", analytics.SaveReportInput{OperationID: testkit.NewUser().ID, PreviewID: preview.ID, Confirm: true}); r.status != 404 {
		t.Fatal("foreign preview", r.status)
	}
	readQuest(t, f.privateRequest(t, a, "POST", "/quests/"+q.ID+"/reopen", version(q), quests.Transition{Reason: "Коррекция недели"}))
	after := readReport(t, f.privateRequest(t, a, "GET", "/analytics/reports/"+saved.ID, "", nil))
	body, _ := json.Marshal(after.Snapshot)
	if string(body) != string(original) {
		t.Fatal("old report rewritten")
	}
	fresh := readReportPreview(t, f.privateRequest(t, a, "POST", "/analytics/reports/preview", "", input))
	if fresh.Snapshot.Comparisons[0].Current != 0 {
		t.Fatal("new report ignores corrections", fresh)
	}
	second := readReport(t, f.privateRequest(t, a, "POST", "/analytics/reports", "", analytics.SaveReportInput{OperationID: testkit.NewUser().ID, PreviewID: fresh.ID, Confirm: true}))
	if second.Revision != 2 {
		t.Fatal(second)
	}
	f.clock.Advance(25 * time.Hour)
	f.loginAs(t, a, "report-snapshot@example.test")
	replay := readReport(t, f.privateRequest(t, a, "POST", "/analytics/reports", "", in))
	if replay.ID != saved.ID {
		t.Fatal("permanent save replay")
	}
	in.PreviewID = fresh.ID
	if r := f.privateRequest(t, a, "POST", "/analytics/reports", "", in); r.status != 409 {
		t.Fatal("operation rebound", r.status)
	}
	if _, e := f.pool.Exec(f.ctx, "UPDATE report_snapshots SET snapshot='{}' WHERE owner_id=$1 AND id=$2", who.ID, saved.ID); e == nil {
		t.Fatal("snapshot mutable")
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM report_snapshots WHERE owner_id=$1", who.ID).Scan(&n); e != nil || n != 2 {
		t.Fatal(n, e)
	}
}
func TestAnalyticsReportStaleExpiredAndIncompleteHistory(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	a, _ := f.signedIn(t, "report-stale@example.test")
	in := analytics.ReportInput{Kind: "week", StartsOn: "2026-10-05", Timezone: "UTC"}
	p := readReportPreview(t, f.privateRequest(t, a, "POST", "/analytics/reports/preview", "", in))
	if p.Snapshot.CurrentObserved || p.Snapshot.PreviousObserved || p.Snapshot.Comparisons[0].Previous != nil || p.Snapshot.Comparisons[0].Percent != nil || !strings.Contains(p.Snapshot.Text, "неполные") {
		t.Fatal("invented history", p)
	}
	q := readQuest(t, f.privateRequest(t, a, "POST", "/quests", "", questInput("Новый источник")))
	_ = q
	save := analytics.SaveReportInput{OperationID: testkit.NewUser().ID, PreviewID: p.ID, Confirm: true}
	if r := f.privateRequest(t, a, "POST", "/analytics/reports", "", save); r.status != 409 {
		t.Fatal("stale preview saved", r.status)
	}
	p = readReportPreview(t, f.privateRequest(t, a, "POST", "/analytics/reports/preview", "", in))
	save.PreviewID = p.ID
	f.clock.Advance(10 * time.Minute)
	if r := f.privateRequest(t, a, "POST", "/analytics/reports", "", save); r.status != 409 {
		t.Fatal("expired preview saved", r.status)
	}
	in.StartsOn = "2026-10-12"
	if r := f.privateRequest(t, a, "POST", "/analytics/reports/preview", "", in); r.status != 409 {
		t.Fatal("open period report", r.status)
	}
}
func TestAnalyticsScheduledReportsUseRealLeasesAndDoNotDuplicate(t *testing.T) {
	f := newAuthFixture(t, auth.DefaultLimits())
	_, who := f.signedIn(t, "report-scheduled@example.test")
	f.clock.Advance(63 * 24 * time.Hour)
	progress := progression.NewService(f.pool, f.clock)
	service := analytics.NewService(f.pool, f.clock).WithRecords(character.NewService(f.pool, f.clock, progress)).WithReportObserver(notifications.NewService(f.pool, f.clock).ReportCreated)
	if e := service.StartReportSchedule(f.ctx); e != nil {
		t.Fatal(e)
	}
	store := jobs.Store{Pool: f.pool, Clock: f.clock}
	sweep, e := store.Claim(f.ctx)
	if e != nil || sweep.Kind != "analytics.reports_sweep" {
		t.Fatal(sweep, e)
	}
	if e = store.Process(f.ctx, sweep, service.Handlers()); e != nil {
		t.Fatal(e)
	}
	due, e := store.Claim(f.ctx)
	if e != nil || due.Kind != "analytics.reports_due" {
		t.Fatal(due, e)
	}
	if e = store.Process(f.ctx, due, service.Handlers()); e != nil {
		t.Fatal(e)
	}
	if e = store.Process(f.ctx, due, service.Handlers()); e != jobs.ErrLeaseLost {
		t.Fatal("repeated lease accepted", e)
	}
	var n int
	if e = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM report_snapshots WHERE owner_id=$1 AND automatic", who.ID).Scan(&n); e != nil || n != 2 {
		t.Fatal("missing automatic week/month", n, e)
	}
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM notifications WHERE owner_id=$1 AND kind='report'`, who.ID).Scan(&n); e != nil || n != 2 {
		t.Fatal("missing automatic report notifications", n, e)
	}
	if e = service.StartReportSchedule(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM jobs WHERE kind='analytics.reports_sweep'").Scan(&n); e != nil || n != 2 {
		t.Fatal("duplicate hourly sweep", n, e)
	}
}
