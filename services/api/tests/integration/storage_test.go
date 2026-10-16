//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/internal/storage"
	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func database(t *testing.T) (*pgxpool.Pool, context.Context) {
	return databaseWithTimeout(t, 30*time.Second)
}

func databaseWithTimeout(t *testing.T, timeout time.Duration) (*pgxpool.Pool, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL must explicitly name an isolated test database")
	}
	u, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal("invalid TEST_DATABASE_URL")
	}
	if u.Path != "/ascend_test" {
		t.Fatal("integration tests require a database named ascend_test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schema := "ascend_test_" + hex.EncodeToString(random[:])
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 60*time.Second)
		defer stop()
		// A full schema now contains hundreds of relations and indexes. Parallel
		// DROP CASCADE operations can exhaust PostgreSQL's shared lock table.
		tx, err := admin.Begin(cleanup)
		if err == nil {
			_, err = tx.Exec(cleanup, "SELECT pg_advisory_xact_lock(210022)")
			if err == nil {
				_, err = tx.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE")
			}
			if err == nil {
				err = tx.Commit(cleanup)
			} else {
				_ = tx.Rollback(cleanup)
			}
		}
		if err != nil {
			t.Errorf("test schema cleanup failed: %v", err)
		}
		_ = admin.Close(cleanup)
	})
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	if err := migrations.Up(ctx, u.String()); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Up(ctx, u.String()); err != nil {
		t.Fatal("migration rerun failed", err)
	}
	pool, err := storage.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, ctx
}

func TestTransactionRollbackAndCommit(t *testing.T) {
	t.Parallel()
	pool, ctx := database(t)
	sentinel := errors.New("reject operation")
	err := storage.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO audit_events (action,entity_type) VALUES ('probe','test')`)
		if err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback left rows: %d, %v", count, err)
	}
	if err := storage.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO audit_events (action,entity_type) VALUES ('probe','test')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("commit rows: %d, %v", count, err)
	}
}
