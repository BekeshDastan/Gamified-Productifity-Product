//go:build integration

package integration

import (
	"database/sql"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/BekeshDastan/Gamified-Productifity-Product/services/api/migrations"
	"github.com/pressly/goose/v3"
)

func TestSchemasAreIsolated(t *testing.T) {
	t.Parallel()
	first, ctx := database(t)
	second, _ := database(t)
	if _, err := first.Exec(ctx, `INSERT INTO audit_events(action,entity_type)VALUES('isolation','test')`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := second.QueryRow(ctx, `SELECT count(*) FROM audit_events`).Scan(&count); err != nil || count != 0 {
		t.Fatal("test schemas leaked data", err)
	}
}
func TestInvalidMigrationDoesNotAdvanceVersion(t *testing.T) {
	t.Parallel()
	pool, ctx := database(t)
	files := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = &fstest.MapFile{Data: data}
	}
	files["99999_invalid.sql"] = &fstest.MapFile{Data: []byte("-- +goose Up\nTHIS IS INVALID SQL;\n-- +goose Down\nSELECT 1;\n")}
	db, err := sql.Open("pgx", pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err == nil {
		t.Fatal("invalid migration accepted")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM goose_db_version WHERE version_id=99999 AND is_applied`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed migration marked applied", err)
	}
}
