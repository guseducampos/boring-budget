package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"budgetto/internal/domain"
)

func TestSettingsRepoUpsertAndGet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openSettingsTestDB(t)
	defer db.Close()

	repo := NewSettingsRepo(db)
	completedAt := time.Now().UTC().Format(time.RFC3339Nano)
	settings, err := repo.Upsert(ctx, domain.SettingsUpsertInput{
		DefaultCurrencyCode:        "usd",
		DisplayTimezone:            "America/New_York",
		OrphanCountThreshold:       7,
		OrphanSpendingThresholdBPS: 600,
		OnboardingCompletedAtUTC:   &completedAt,
	})
	if err != nil {
		t.Fatalf("upsert settings: %v", err)
	}

	if settings.DefaultCurrencyCode != "USD" {
		t.Fatalf("expected default currency USD, got %q", settings.DefaultCurrencyCode)
	}
	if settings.DisplayTimezone != "America/New_York" {
		t.Fatalf("unexpected timezone: %q", settings.DisplayTimezone)
	}
	if settings.OrphanCountThreshold != 7 || settings.OrphanSpendingThresholdBPS != 600 {
		t.Fatalf("unexpected thresholds: %+v", settings)
	}
	if settings.OnboardingCompletedAtUTC == nil {
		t.Fatalf("expected onboarding timestamp")
	}

	loaded, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if loaded.ID != 1 {
		t.Fatalf("expected singleton settings id=1, got %d", loaded.ID)
	}
}

func openSettingsTestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "settings-test.db")
	db, err := OpenAndMigrate(ctx, dbPath, migrationsDirFromSettingsRepoTest(t))
	if err != nil {
		t.Fatalf("open and migrate test db: %v", err)
	}
	return db
}

func migrationsDirFromSettingsRepoTest(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current file path")
	}

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	return filepath.Join(projectRoot, "migrations")
}
