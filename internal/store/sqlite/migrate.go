package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func RunMigrations(ctx context.Context, db *sql.DB, migrationsDir string) error {
	if db == nil {
		return fmt.Errorf("run migrations: db is nil")
	}

	if migrationsDir == "" {
		migrationsDir = DefaultMigrationsDir
	}

	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose sqlite dialect: %w", err)
	}

	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
