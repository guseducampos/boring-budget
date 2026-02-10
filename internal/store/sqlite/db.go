package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

const (
	DriverName           = "sqlite"
	DefaultMigrationsDir = "migrations"
)

// Open initializes a SQLite connection with required pragmas for this app.
func Open(ctx context.Context, dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		return nil, errors.New("sqlite open: db path is required")
	}

	db, err := sql.Open(DriverName, dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}

	// Keep a single connection so connection-local pragmas stay consistent.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite ping: %w", err)
	}

	if err := applyPragmas(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

// OpenAndMigrate opens the database and applies migrations from migrationsDir.
func OpenAndMigrate(ctx context.Context, dbPath, migrationsDir string) (*sql.DB, error) {
	db, err := Open(ctx, dbPath)
	if err != nil {
		return nil, err
	}

	if migrationsDir == "" {
		migrationsDir = DefaultMigrationsDir
	}

	if err := RunMigrations(ctx, db, migrationsDir); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func applyPragmas(ctx context.Context, db *sql.DB) error {
	statements := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
	}

	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite pragma %q: %w", stmt, err)
		}
	}

	return nil
}
