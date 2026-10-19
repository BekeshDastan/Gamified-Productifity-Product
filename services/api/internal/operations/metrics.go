package operations

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Handler(pool *pgxpool.Pool, token, directory string) (http.Handler, error) {
	if len(token) < 43 {
		return nil, errors.New("OPS_BEARER_TOKEN must contain at least 32 random bytes encoded as base64")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" || r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		var pending, dead, expired, outbox int64
		var lag, heartbeat, outboxLag float64
		err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state IN('pending','retry_wait')),count(*) FILTER(WHERE state='dead'),count(*) FILTER(WHERE state='running' AND lease_until<now()),COALESCE(max(EXTRACT(epoch FROM now()-next_run_at)) FILTER(WHERE state IN('pending','retry_wait') AND next_run_at<now()),0) FROM jobs WHERE state IN('pending','retry_wait','dead','running')`).Scan(&pending, &dead, &expired, &lag)
		if err == nil {
			err = pool.QueryRow(ctx, `SELECT COALESCE((SELECT EXTRACT(epoch FROM now()-seen_at) FROM runtime_heartbeats WHERE name='worker'),1000000000)`).Scan(&heartbeat)
		}
		if err == nil {
			err = pool.QueryRow(ctx, `SELECT count(*),COALESCE(max(EXTRACT(epoch FROM now()-occurred_at)),0) FROM outbox_events WHERE dispatched_at IS NULL`).Scan(&outbox, &outboxLag)
		}
		if err != nil {
			w.WriteHeader(503)
			fmt.Fprintln(w, "ascend_database_up 0")
			return
		}
		var out strings.Builder
		out.WriteString(platform.HTTPMetrics())
		fmt.Fprintf(&out, "ascend_database_up 1\nascend_jobs_pending %d\nascend_jobs_dead %d\nascend_jobs_expired_leases %d\nascend_jobs_lag_seconds %g\nascend_worker_heartbeat_age_seconds %g\n", pending, dead, expired, lag, heartbeat)
		fmt.Fprintf(&out, "ascend_outbox_pending %d\nascend_outbox_lag_seconds %g\n", outbox, outboxLag)
		if free, total, ok := diskSpace(directory); ok {
			fmt.Fprintf(&out, "ascend_storage_available_bytes %d\nascend_storage_total_bytes %d\n", free, total)
		}
		fmt.Fprint(w, out.String())
	}), nil
}
