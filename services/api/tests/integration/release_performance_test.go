//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/auth"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/planning"
)

// Explicit load fixture: never seeds a working account or production database.
func TestReleasePerformance(t *testing.T) {
	if os.Getenv("RUN_RELEASE_PERF") != "1" {
		t.Skip("opt in with RUN_RELEASE_PERF=1")
	}
	limits := auth.DefaultLimits()
	limits.UserRequests = 10000
	f := newAuthFixture(t, limits)
	client, account := f.signedIn(t, "release-perf@example.test")
	input := questInput("Load fixture")
	payload, _ := json.Marshal(input)
	_, err := f.pool.Exec(f.ctx, `INSERT INTO quests(owner_id,payload,status,created_at,updated_at) SELECT $1,$2,'todo',now()-n*interval '1 minute',now() FROM generate_series(1,10000) n`, account.ID, payload)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := f.pool.Query(f.ctx, `SELECT id::text FROM quests WHERE owner_id=$1 ORDER BY id LIMIT 50`, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for selected.Next() {
		var id string
		if err := selected.Scan(&id); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, id)
	}
	if err := selected.Err(); err != nil {
		t.Fatal(err)
	}
	selected.Close()
	if _, err = f.pool.Exec(f.ctx, `UPDATE quests SET planned_date='2026-10-12',payload=payload||'{"planned_date":"2026-10-12"}'::jsonb WHERE owner_id=$1 AND id=ANY($2::uuid[])`, account.ID, keys); err != nil {
		t.Fatal(err)
	}
	plan := planning.Input{AvailableMinutes: 480, QuestIDs: keys, TopIDs: keys[:3], Windows: []planning.Window{}}
	if response := f.privateRequest(t, client, "PUT", "/plans/day/2026-10-12", `"0"`, plan); response.status != 200 {
		t.Fatalf("plan fixture: %d %s", response.status, response.body)
	}
	if _, err = f.pool.Exec(f.ctx, `ANALYZE quests`); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/dashboard?date=2026-10-12", "/quests?page_size=50", "/calendar?from=2026-10-12&to=2026-10-19"} {
		const count = 100
		const concurrency = 4
		samples := make([]float64, count)
		failures := make(chan string, count)
		var wg sync.WaitGroup
		for worker := 0; worker < concurrency; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for i := worker; i < count; i += concurrency {
					start := time.Now()
					response, err := client.Get(f.server.URL + "/api/v1" + route)
					if err != nil {
						failures <- "transport"
						continue
					}
					_, readErr := io.Copy(io.Discard, response.Body)
					response.Body.Close()
					if response.StatusCode != http.StatusOK || readErr != nil {
						failures <- fmt.Sprintf("status %d", response.StatusCode)
					}
					samples[i] = float64(time.Since(start).Microseconds()) / 1000
				}
			}(worker)
		}
		wg.Wait()
		close(failures)
		for failure := range failures {
			t.Fatal(route, failure)
		}
		sort.Float64s(samples)
		t.Logf("PERF route=%s rows=10000 requests=%d concurrency=%d p50_ms=%.2f p95_ms=%.2f p99_ms=%.2f", route, count, concurrency, samples[49], samples[94], samples[98])
	}
	rows, err := f.pool.Query(f.ctx, `EXPLAIN (ANALYZE,BUFFERS,FORMAT TEXT) SELECT q.id FROM quests q WHERE q.owner_id=$1 AND q.archived_at IS NULL AND NOT EXISTS(SELECT 1 FROM quests c WHERE c.owner_id=q.owner_id AND c.parent_id=q.id AND c.archived_at IS NULL) AND q.planned_date='2026-10-12' ORDER BY q.deadline NULLS LAST,q.created_at,q.id LIMIT 101`, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		t.Log(line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
