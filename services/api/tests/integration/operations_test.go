//go:build integration

package integration

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/operations"
)

func TestOperationsDetectsStoppedWorkerAndDeadJobs(t *testing.T) {
	pool, ctx := database(t)
	token := strings.Repeat("a", 43)
	handler, err := operations.Handler(pool, token, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	read := func() string {
		r := httptest.NewRequest("GET", "/metrics", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		return w.Body.String()
	}
	if !strings.Contains(read(), "ascend_worker_heartbeat_age_seconds 1e+09") {
		t.Fatal("missing worker not detected")
	}
	_, err = pool.Exec(ctx, `INSERT INTO runtime_heartbeats VALUES('worker',now()-interval '2 minutes');INSERT INTO jobs(kind,payload,dedupe_key,state,next_run_at,created_at) VALUES('private.test','{"secret":"never-output"}','ops-failure','dead',now(),now())`)
	if err != nil {
		t.Fatal(err)
	}
	body := read()
	if !strings.Contains(body, "ascend_jobs_dead 1") || strings.Contains(body, "never-output") || strings.Contains(body, "private.test") {
		t.Fatal("metrics missing dead job or leaking payload")
	}
	_, err = pool.Exec(ctx, `UPDATE runtime_heartbeats SET seen_at=now();UPDATE jobs SET state='cancelled' WHERE dedupe_key='ops-failure'`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(), "ascend_jobs_dead 0") {
		t.Fatal("recovery not reflected")
	}
}
