package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"boring-budget/internal/domain"
)

func TestAuditTriggersRecordKeyMutations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openAuditTriggerTestDB(t)
	defer db.Close()

	categoryRepo := NewCategoryRepo(db)
	labelRepo, err := NewLabelRepo(db)
	if err != nil {
		t.Fatalf("new label repo: %v", err)
	}
	entryRepo := NewEntryRepo(db)
	capRepo := NewCapRepo(db)
	settingsRepo := NewSettingsRepo(db)

	category, err := categoryRepo.Add(ctx, "Food")
	if err != nil {
		t.Fatalf("add category: %v", err)
	}
	if _, err := categoryRepo.Rename(ctx, category.ID, "Groceries"); err != nil {
		t.Fatalf("rename category: %v", err)
	}
	if _, err := categoryRepo.SoftDelete(ctx, category.ID); err != nil {
		t.Fatalf("delete category: %v", err)
	}

	label, err := labelRepo.Add(ctx, "Recurring")
	if err != nil {
		t.Fatalf("add label: %v", err)
	}
	if _, err := labelRepo.Rename(ctx, label.ID, "Fixed"); err != nil {
		t.Fatalf("rename label: %v", err)
	}
	if _, err := labelRepo.Delete(ctx, label.ID); err != nil {
		t.Fatalf("delete label: %v", err)
	}

	entry, err := entryRepo.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        1000,
		CurrencyCode:       "USD",
		TransactionDateUTC: time.Now().UTC().Format(time.RFC3339Nano),
		Note:               "trigger test",
	})
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if _, err := entryRepo.Delete(ctx, entry.ID); err != nil {
		t.Fatalf("delete entry: %v", err)
	}

	if _, _, err := capRepo.Set(ctx, domain.CapSetInput{MonthKey: "2026-02", AmountMinor: 5000, CurrencyCode: "USD"}); err != nil {
		t.Fatalf("set cap: %v", err)
	}

	onboarding := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := settingsRepo.Upsert(ctx, domain.SettingsUpsertInput{
		DefaultCurrencyCode:      "USD",
		DisplayTimezone:          "UTC",
		OnboardingCompletedAtUTC: &onboarding,
	}); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}

	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events;").Scan(&count); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if count == 0 {
		t.Fatalf("expected audit events to be recorded")
	}
}

func openAuditTriggerTestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "audit-triggers-test.db")
	db, err := OpenAndMigrate(ctx, dbPath, migrationsDirFromAuditTriggerTest(t))
	if err != nil {
		t.Fatalf("open and migrate test db: %v", err)
	}
	return db
}

func migrationsDirFromAuditTriggerTest(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current file path")
	}

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	return filepath.Join(projectRoot, "migrations")
}
