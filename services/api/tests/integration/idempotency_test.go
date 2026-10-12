//go:build integration

package integration

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/jackc/pgx/v5"
)

func TestIdempotencyScopesAndConcurrentReplay(t *testing.T) {
	t.Parallel()
	pool, ctx := database(t)
	store := storage.IdempotencyStore{DB: pool, Now: func() time.Time { return time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC) }}
	scope := storage.Scope{UserID: "00000000-0000-4000-8000-000000000001", Method: "POST", RouteKey: "/api/v1/tasks/task-a/complete", Key: "operation-key-0001"}
	var calls atomic.Int32
	operation := func(tx pgx.Tx) (storage.Result, error) {
		calls.Add(1)
		_, err := tx.Exec(ctx, `INSERT INTO audit_events(action,entity_type)VALUES('idempotency','test')`)
		return storage.Result{Status: 200, Body: []byte(`{"ok":true}`)}, err
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.Execute(ctx, scope, []byte(`{}`), operation)
			if err != nil || result.Status != 200 {
				t.Errorf("concurrent operation: %v", err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("operation ran %d times", calls.Load())
	}
	if _, err := store.Execute(ctx, scope, []byte(`{"different":true}`), operation); !errors.Is(err, storage.ErrIdempotencyMismatch) {
		t.Fatal("body mismatch accepted", err)
	}
	scope.UserID = "00000000-0000-4000-8000-000000000002"
	if _, err := store.Execute(ctx, scope, []byte(`{}`), operation); err != nil {
		t.Fatal(err)
	}
	scope.RouteKey = "/api/v1/tasks/task-b/complete"
	if _, err := store.Execute(ctx, scope, []byte(`{}`), operation); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("users or resources shared cached results")
	}
	scope.Key = "invalid"
	if _, err := store.Execute(ctx, scope, nil, operation); !errors.Is(err, storage.ErrInvalidScope) {
		t.Fatal("invalid key accepted")
	}
}
