//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/platform/jobs"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/jackc/pgx/v5"
)

func TestOutboxDedupeLeaseRecoveryAndRetry(t *testing.T) {
	t.Parallel()
	pool, ctx := database(t)
	clock := &platform.FixedClock{Time: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
	store := jobs.Store{Pool: pool, Clock: clock, Jitter: func(d time.Duration) time.Duration { return d }}
	event := jobs.Event{ID: "00000000-0000-4000-8000-000000000001", Kind: "test.effect", Payload: []byte(`{}`)}
	for range 2 {
		if err := storage.WithinTx(ctx, pool, func(tx pgx.Tx) error { return store.AddOutbox(ctx, tx, event) }); err != nil {
			t.Fatal(err)
		}
	}
	count, err := store.Dispatch(ctx, 50)
	if err != nil || count != 1 {
		t.Fatalf("dispatch: %d %v", count, err)
	}
	count, err = store.Dispatch(ctx, 50)
	if err != nil || count != 0 {
		t.Fatal("outbox dispatched twice")
	}
	old, err := store.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	clock.Time = clock.Time.Add(31 * time.Second)
	current, err := store.Claim(ctx)
	if err != nil || current.ID != old.ID || current.LeaseToken == old.LeaseToken {
		t.Fatal("expired lease not recovered", err)
	}
	handlers := map[string]jobs.Handler{"test.effect": func(ctx context.Context, tx pgx.Tx, _ jobs.Job) error {
		_, err := tx.Exec(ctx, `INSERT INTO audit_events(action,entity_type)VALUES('job.effect','test')`)
		return err
	}}
	if err := store.Process(ctx, old, handlers); !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatal("old worker retained lease", err)
	}
	if err := store.Process(ctx, current, handlers); err != nil {
		t.Fatal(err)
	}
	if err := store.Process(ctx, current, handlers); !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatal("completed job ran again")
	}
	var effects int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='job.effect'`).Scan(&effects); err != nil || effects != 1 {
		t.Fatal("duplicate job effect", effects, err)
	}
	event.ID = "00000000-0000-4000-8000-000000000002"
	if err := storage.WithinTx(ctx, pool, func(tx pgx.Tx) error { return store.Enqueue(ctx, tx, event, "retry-test") }); err != nil {
		t.Fatal(err)
	}
	retry, err := store.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	transient := errors.New("private provider details")
	if err := store.Process(ctx, retry, map[string]jobs.Handler{"test.effect": func(context.Context, pgx.Tx, jobs.Job) error { return transient }}); !errors.Is(err, transient) {
		t.Fatal(err)
	}
	if _, err := store.Claim(ctx); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("retry ran before backoff")
	}
	clock.Time = clock.Time.Add(11 * time.Second)
	retry, err = store.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Process(ctx, retry, map[string]jobs.Handler{}); !errors.Is(err, jobs.ErrPermanent) {
		t.Fatal("unknown handler was not permanent")
	}
	var state, code string
	if err := pool.QueryRow(ctx, `SELECT state,last_error_code FROM jobs WHERE id=$1`, retry.ID).Scan(&state, &code); err != nil || state != "dead" || code == transient.Error() {
		t.Fatal("dead letter or redaction failed")
	}
}
