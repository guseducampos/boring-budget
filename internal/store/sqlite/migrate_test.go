package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenSetsExpectedPragmas(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		t.Fatalf("query journal mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("expected journal_mode=wal, got %q", journalMode)
	}

	var foreignKeys int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&foreignKeys); err != nil {
		t.Fatalf("query foreign_keys pragma: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("expected foreign_keys=1, got %d", foreignKeys)
	}
}

func TestRunMigrationsIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	migrationsDir := filepath.Join(tempDir, "migrations")

	if err := writeFile(filepath.Join(migrationsDir, "0001_create_demo.sql"), `
CREATE TABLE IF NOT EXISTS demo_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL
);`); err != nil {
		t.Fatalf("write migration 0001: %v", err)
	}

	if err := writeFile(filepath.Join(migrationsDir, "0002_create_other.sql"), `
CREATE TABLE IF NOT EXISTS other_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	value TEXT NOT NULL
);`); err != nil {
		t.Fatalf("write migration 0002: %v", err)
	}

	db, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := RunMigrations(ctx, db, migrationsDir); err != nil {
		t.Fatalf("run migrations first pass: %v", err)
	}
	assertTableExists(t, ctx, db, "demo_items")
	assertTableExists(t, ctx, db, "other_items")
	assertMigrationCount(t, ctx, db, 2)

	if err := RunMigrations(ctx, db, migrationsDir); err != nil {
		t.Fatalf("run migrations second pass: %v", err)
	}
	assertMigrationCount(t, ctx, db, 2)
}

func assertMigrationCount(t *testing.T, ctx context.Context, db *sql.DB, expected int) {
	t.Helper()

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations;").Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != expected {
		t.Fatalf("expected %d applied migrations, got %d", expected, count)
	}
}

func assertTableExists(t *testing.T, ctx context.Context, db *sql.DB, table string) {
	t.Helper()

	var name string
	if err := db.QueryRowContext(
		ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name = ?;",
		table,
	).Scan(&name); err != nil {
		t.Fatalf("expected table %q to exist: %v", table, err)
	}
}

func writeFile(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
