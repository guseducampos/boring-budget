package sqlite

import (
	"boring-budget/migrations"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"

	"github.com/pressly/goose/v3"
)

var gooseMu sync.Mutex

func RunMigrations(ctx context.Context, db *sql.DB, migrationsDir string) error {
	if db == nil {
		return fmt.Errorf("run migrations: db is nil")
	}

	dir, baseFS, err := resolveMigrationSource(migrationsDir)
	if err != nil {
		return err
	}

	gooseMu.Lock()
	defer gooseMu.Unlock()

	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(baseFS)
	defer goose.SetBaseFS(nil)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose sqlite dialect: %w", err)
	}

	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}

func resolveMigrationSource(migrationsDir string) (string, fs.FS, error) {
	if migrationsDir == "" {
		migrationsDir = DefaultMigrationsDir
	}

	if migrationsDir != DefaultMigrationsDir {
		return migrationsDir, nil, nil
	}

	if _, err := os.Stat(migrationsDir); err == nil {
		return migrationsDir, nil, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", nil, fmt.Errorf("stat migrations dir %q: %w", migrationsDir, err)
	}

	return ".", migrations.FS, nil
}
