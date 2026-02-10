package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func RunMigrations(ctx context.Context, db *sql.DB, migrationsDir string) error {
	if db == nil {
		return fmt.Errorf("run migrations: db is nil")
	}

	if migrationsDir == "" {
		migrationsDir = DefaultMigrationsDir
	}

	if err := ensureMigrationsTable(ctx, db); err != nil {
		return err
	}

	files, err := listMigrationFiles(migrationsDir)
	if err != nil {
		return err
	}

	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return err
	}

	for _, file := range files {
		if _, ok := applied[file]; ok {
			continue
		}

		path := filepath.Join(migrationsDir, file)
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %q: %w", path, err)
		}

		if strings.TrimSpace(string(content)) == "" {
			if err := recordMigration(ctx, db, file); err != nil {
				return err
			}
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %q: %w", file, err)
		}

		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("execute migration %q: %w", file, err)
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO schema_migrations (filename, applied_at_utc) VALUES (?, ?)`,
			file,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %q: %w", file, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %q: %w", file, err)
		}
	}

	return nil
}

func ensureMigrationsTable(ctx context.Context, db *sql.DB) error {
	const stmt = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	filename TEXT PRIMARY KEY,
	applied_at_utc TEXT NOT NULL
);`
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}
	return nil
}

func listMigrationFiles(migrationsDir string) ([]string, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir %q: %w", migrationsDir, err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".sql" {
			continue
		}
		files = append(files, name)
	}

	sort.Strings(files)
	return files, nil
}

func appliedMigrations(ctx context.Context, db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, `SELECT filename FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()

	out := make(map[string]struct{})
	for rows.Next() {
		var filename string
		if err := rows.Scan(&filename); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		out[filename] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}

	return out, nil
}

func recordMigration(ctx context.Context, db *sql.DB, file string) error {
	_, err := db.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (filename, applied_at_utc) VALUES (?, ?)`,
		file,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("record empty migration %q: %w", file, err)
	}
	return nil
}
